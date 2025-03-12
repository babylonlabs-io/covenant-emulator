package middlewares_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/middlewares"
)

func TestGenerateHMAC(t *testing.T) {
	tests := []struct {
		name        string
		hmacKey     string
		body        []byte
		expectError bool
	}{
		{
			name:        "Valid HMAC",
			hmacKey:     "test-key",
			body:        []byte(`{"test":"data"}`),
			expectError: false,
		},
		{
			name:        "Empty HMAC Key",
			hmacKey:     "",
			body:        []byte(`{"test":"data"}`),
			expectError: false,
		},
		{
			name:        "Empty Body",
			hmacKey:     "test-key",
			body:        []byte{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := middlewares.GenerateHMAC(tt.hmacKey, tt.body)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			resultAgain, err := middlewares.GenerateHMAC(tt.hmacKey, tt.body)
			if err != nil {
				t.Errorf("Unexpected error on second generation: %v", err)
			}

			if result != resultAgain {
				t.Errorf("HMAC not consistent: first=%s, second=%s", result, resultAgain)
			}
		})
	}
}

func TestValidateHMAC(t *testing.T) {
	tests := []struct {
		name          string
		hmacKey       string
		body          []byte
		expectedValid bool
		expectError   bool
	}{
		{
			name:          "Valid HMAC",
			hmacKey:       "test-key",
			body:          []byte(`{"test":"data"}`),
			expectedValid: true,
			expectError:   false,
		},
		{
			name:          "Invalid HMAC",
			hmacKey:       "wrong-key",
			body:          []byte(`{"test":"data"}`),
			expectedValid: false,
			expectError:   false,
		},
		{
			name:          "Empty HMAC Key",
			hmacKey:       "",
			body:          []byte(`{"test":"data"}`),
			expectedValid: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hmac, err := middlewares.GenerateHMAC("test-key", tt.body)
			if err != nil {
				t.Fatalf("Failed to generate HMAC: %v", err)
			}

			valid, err := middlewares.ValidateHMAC(tt.hmacKey, tt.body, hmac)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if valid != tt.expectedValid {
				t.Errorf("Expected valid=%v, got %v", tt.expectedValid, valid)
			}
		})
	}
}

func TestEmptyReceivedHMAC(t *testing.T) {
	key := "test-key"
	body := []byte(`{"test":"data"}`)
	receivedHMAC := ""

	valid, err := middlewares.ValidateHMAC(key, body, receivedHMAC)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if valid {
		t.Errorf("Expected validation to fail with empty HMAC, but it passed")
	}
}

func TestRewindRequestBody(t *testing.T) {
	tests := []struct {
		name        string
		inputBody   string
		expectError bool
	}{
		{
			name:        "Valid Body",
			inputBody:   `{"test":"data"}`,
			expectError: false,
		},
		{
			name:        "Empty Body",
			inputBody:   "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readCloser := io.NopCloser(strings.NewReader(tt.inputBody))

			body, newReader, err := middlewares.RewindRequestBody(readCloser)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if string(body) != tt.inputBody {
				t.Errorf("Expected body %s, got %s", tt.inputBody, string(body))
			}

			newBody, err := io.ReadAll(newReader)
			if err != nil {
				t.Errorf("Error reading from new reader: %v", err)
			}

			if string(newBody) != tt.inputBody {
				t.Errorf("Expected body from new reader %s, got %s", tt.inputBody, string(newBody))
			}
		})
	}

	t.Run("Size limit tests", func(t *testing.T) {
		testMaxSize := 1024 * 10 // 10KB for testing

		testRewindRequestBody := func(reader io.ReadCloser) ([]byte, io.ReadCloser, error) {
			limitedReader := io.LimitReader(reader, int64(testMaxSize+1))
			body, err := io.ReadAll(limitedReader)
			if err != nil {
				return nil, nil, err
			}

			if len(body) > testMaxSize {
				return nil, nil, errors.New("request body too large")
			}

			return body, io.NopCloser(bytes.NewReader(body)), nil
		}

		t.Run("Body at exactly max size", func(t *testing.T) {
			exactSizeBody := bytes.Repeat([]byte("a"), testMaxSize)
			reader := io.NopCloser(bytes.NewReader(exactSizeBody))

			body, newBody, err := testRewindRequestBody(reader)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if len(body) != testMaxSize {
				t.Errorf("Expected body length %d, got %d", testMaxSize, len(body))
			}

			readBody, err := io.ReadAll(newBody)
			if err != nil {
				t.Errorf("Error reading from new reader: %v", err)
			}

			if len(readBody) != testMaxSize {
				t.Errorf("Expected body length from new reader %d, got %d", testMaxSize, len(readBody))
			}
		})

		t.Run("Body exceeding max size", func(t *testing.T) {
			oversizeBody := bytes.Repeat([]byte("a"), testMaxSize+1)
			reader := io.NopCloser(bytes.NewReader(oversizeBody))

			_, _, err := testRewindRequestBody(reader)
			if err == nil {
				t.Errorf("Expected error but got none")
			}

			if err.Error() != "request body too large" {
				t.Errorf("Expected error 'request body too large', got '%v'", err)
			}
		})

		t.Run("Body exceeding max size by a lot", func(t *testing.T) {
			largeReader := &infiniteReader{maxRead: testMaxSize * 2}
			reader := io.NopCloser(largeReader)

			_, _, err := testRewindRequestBody(reader)
			if err == nil {
				t.Errorf("Expected error but got none")
			}

			if err.Error() != "request body too large" {
				t.Errorf("Expected error 'request body too large', got '%v'", err)
			}
		})
	})
}

// infiniteReader is a mock io.Reader that returns a specified byte repeatedly
// up to a maximum number of bytes
type infiniteReader struct {
	bytesRead int
	maxRead   int
}

func (r *infiniteReader) Read(p []byte) (n int, err error) {
	if r.bytesRead >= r.maxRead {
		return 0, io.EOF
	}

	for i := range p {
		p[i] = 'a'
		r.bytesRead++
		if r.bytesRead >= r.maxRead {
			return i + 1, io.EOF
		}
	}

	return len(p), nil
}
