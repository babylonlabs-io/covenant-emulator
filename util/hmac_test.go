package util_test

import (
	"github.com/babylonlabs-io/covenant-emulator/util"
	"io"
	"strings"
	"testing"
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
			result, err := util.GenerateHMAC(tt.hmacKey, tt.body)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			resultAgain, err := util.GenerateHMAC(tt.hmacKey, tt.body)
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
			hmac, err := util.GenerateHMAC("test-key", tt.body)
			if err != nil {
				t.Fatalf("Failed to generate HMAC: %v", err)
			}

			valid, err := util.ValidateHMAC(tt.hmacKey, tt.body, hmac)

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

	valid, err := util.ValidateHMAC(key, body, receivedHMAC)
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

			body, newReader, err := util.RewindRequestBody(readCloser)

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
}
