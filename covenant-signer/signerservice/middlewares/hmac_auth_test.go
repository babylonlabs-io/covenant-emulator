package middlewares_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/middlewares"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/types"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHMACAuthMiddleware(t *testing.T) {
	testHMACKey := "test-hmac-secret-key"
	testBody := []byte(`{"test":"data"}`)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("success"))
		if err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	})

	makeRequestWithHMAC := func(_ *testing.T, body []byte, _, hmacHeader string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
		if hmacHeader != "" {
			req.Header.Set(middlewares.HeaderCovenantHMAC, hmacHeader)
		}

		return req
	}

	makeRequestWithMethodAndHMAC := func(_ *testing.T, method string, body []byte, _, hmacHeader string) *http.Request {
		req := httptest.NewRequest(method, "/test", bytes.NewReader(body))
		if hmacHeader != "" {
			req.Header.Set(middlewares.HeaderCovenantHMAC, hmacHeader)
		}

		return req
	}

	executeMiddleware := func(_ *testing.T, middleware func(http.Handler) http.Handler, req *http.Request) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		handler := middleware(nextHandler)
		handler.ServeHTTP(rr, req)

		return rr
	}

	parseErrorResponse := func(t *testing.T, body io.Reader) map[string]interface{} {
		var respMap map[string]interface{}
		err := json.NewDecoder(body).Decode(&respMap)
		require.NoError(t, err, "Failed to decode error response")

		return respMap
	}

	t.Run("HMAC key not configured, should skip verification", func(t *testing.T) {
		middleware := middlewares.HMACAuthMiddleware("")

		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(testBody))

		rr := executeMiddleware(t, middleware, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "success", rr.Body.String())
	})

	t.Run("Missing HMAC header, should return unauthorized", func(t *testing.T) {
		middleware := middlewares.HMACAuthMiddleware(testHMACKey)

		req := makeRequestWithHMAC(t, testBody, testHMACKey, "")

		rr := executeMiddleware(t, middleware, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)

		respMap := parseErrorResponse(t, rr.Body)
		errorMap, ok := respMap["error"].(map[string]interface{})
		require.True(t, ok, "Error field missing in response")

		assert.Equal(t, string(types.Unauthorized), errorMap["code"])
		assert.Contains(t, errorMap["message"], "missing HMAC authentication header")
	})

	t.Run("Invalid HMAC, should return unauthorized", func(t *testing.T) {
		middleware := middlewares.HMACAuthMiddleware(testHMACKey)

		req := makeRequestWithHMAC(t, testBody, testHMACKey, "invalid-hmac")

		rr := executeMiddleware(t, middleware, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)

		respMap := parseErrorResponse(t, rr.Body)
		errorMap, ok := respMap["error"].(map[string]interface{})
		require.True(t, ok, "Error field missing in response")

		assert.Equal(t, string(types.Unauthorized), errorMap["code"])
		assert.Contains(t, errorMap["message"], "invalid HMAC authentication")
	})

	t.Run("Valid HMAC, should proceed to next handler", func(t *testing.T) {
		middleware := middlewares.HMACAuthMiddleware(testHMACKey)

		validHMAC, err := middlewares.GenerateHMAC(testHMACKey, testBody)
		require.NoError(t, err, "Failed to generate HMAC")

		req := makeRequestWithHMAC(t, testBody, testHMACKey, validHMAC)

		rr := executeMiddleware(t, middleware, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "success", rr.Body.String())
	})

	t.Run("Request body can be read again after middleware", func(t *testing.T) {
		middleware := middlewares.HMACAuthMiddleware(testHMACKey)

		validHMAC, err := middlewares.GenerateHMAC(testHMACKey, testBody)
		require.NoError(t, err, "Failed to generate HMAC")

		req := makeRequestWithHMAC(t, testBody, testHMACKey, validHMAC)

		bodyCheckHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err, "Failed to read request body")
			assert.Equal(t, testBody, body, "Request body was not properly preserved")
			w.WriteHeader(http.StatusOK)
		})

		rr := httptest.NewRecorder()
		handler := middleware(bodyCheckHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Broken request body, should return internal error", func(t *testing.T) {
		middleware := middlewares.HMACAuthMiddleware(testHMACKey)

		brokenBodyReader := &brokenReader{}
		req := httptest.NewRequest(http.MethodPost, "/test", brokenBodyReader)
		req.Header.Set(middlewares.HeaderCovenantHMAC, "some-hmac")

		rr := executeMiddleware(t, middleware, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)

		respMap := parseErrorResponse(t, rr.Body)
		errorMap, ok := respMap["error"].(map[string]interface{})
		require.True(t, ok, "Error field missing in response")

		assert.Equal(t, string(types.InternalServiceError), errorMap["code"])
	})

	t.Run("Different HTTP methods", func(t *testing.T) {
		methods := []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodPatch,
		}

		for _, method := range methods {
			t.Run(method, func(t *testing.T) {
				// Create middleware with key
				middleware := middlewares.HMACAuthMiddleware(testHMACKey)

				// Generate valid HMAC
				validHMAC, err := middlewares.GenerateHMAC(testHMACKey, testBody)
				require.NoError(t, err, "Failed to generate HMAC")

				// Create request with valid HMAC and specific method
				req := makeRequestWithMethodAndHMAC(t, method, testBody, testHMACKey, validHMAC)

				// Execute middleware
				rr := executeMiddleware(t, middleware, req)

				// Verify response
				assert.Equal(t, http.StatusOK, rr.Code, "Failed for method: %s", method)
				assert.Equal(t, "success", rr.Body.String(), "Failed for method: %s", method)
			})
		}
	})

	t.Run("Public Key endpoint requires HMAC authentication", func(t *testing.T) {
		middleware := middlewares.HMACAuthMiddleware(testHMACKey)

		// Create request for public-key endpoint without HMAC header
		req := httptest.NewRequest(http.MethodGet, "/v1/public-key", bytes.NewReader([]byte{}))

		// Execute middleware
		rr := executeMiddleware(t, middleware, req)

		// Should fail due to missing HMAC
		assert.Equal(t, http.StatusUnauthorized, rr.Code)

		respMap := parseErrorResponse(t, rr.Body)
		errorMap, ok := respMap["error"].(map[string]interface{})
		require.True(t, ok, "Error field missing in response")

		assert.Equal(t, string(types.Unauthorized), errorMap["code"])
		assert.Contains(t, errorMap["message"], "missing HMAC authentication header")
	})

	t.Run("Public Key endpoint with valid HMAC should pass", func(t *testing.T) {
		middleware := middlewares.HMACAuthMiddleware(testHMACKey)

		// Create GET request for public-key endpoint with valid HMAC
		body := []byte{}
		validHMAC, err := middlewares.GenerateHMAC(testHMACKey, body)
		require.NoError(t, err, "Failed to generate HMAC")

		req := httptest.NewRequest(http.MethodGet, "/v1/public-key", bytes.NewReader(body))
		req.Header.Set(middlewares.HeaderCovenantHMAC, validHMAC)

		// Execute middleware
		rr := executeMiddleware(t, middleware, req)

		// Should succeed with valid HMAC
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "success", rr.Body.String())
	})
}

// brokenReader is a mock io.ReadCloser that always returns an error when Read is called
type brokenReader struct{}

func (b *brokenReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (b *brokenReader) Close() error {
	return nil
}

// mockFailingResponseWriter is a mock http.ResponseWriter that fails during JSON encoding
type mockFailingResponseWriter struct {
	headers     http.Header
	statusCode  int
	writtenData []byte
	writeFailed bool
}

func (m *mockFailingResponseWriter) Header() http.Header {
	return m.headers
}

func (m *mockFailingResponseWriter) Write(data []byte) (int, error) {
	if !m.writeFailed {
		m.writeFailed = true

		return 0, errors.New("simulated write error")
	}
	m.writtenData = data

	return len(data), nil
}

func (m *mockFailingResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

func TestRespondWithError(t *testing.T) {
	t.Run("Standard error response", func(t *testing.T) {
		testErr := types.NewUnauthorizedError("test error message")

		rr := httptest.NewRecorder()

		middlewares.RespondWithError(rr, testErr)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

		var respMap map[string]interface{}
		err := json.NewDecoder(rr.Body).Decode(&respMap)
		require.NoError(t, err, "Failed to decode error response")

		errorMap, ok := respMap["error"].(map[string]interface{})
		require.True(t, ok, "Error field missing in response")

		assert.Equal(t, string(types.Unauthorized), errorMap["code"])
		assert.Equal(t, "test error message", errorMap["message"])
	})

	t.Run("JSON encoding failure", func(t *testing.T) {
		testErr := types.NewInternalServiceError(errors.New("test error"))

		mockResponseWriter := &mockFailingResponseWriter{
			headers: http.Header{},
		}

		middlewares.RespondWithError(mockResponseWriter, testErr)

		assert.Equal(t, http.StatusInternalServerError, mockResponseWriter.statusCode)
		assert.Equal(t, "text/plain", mockResponseWriter.headers.Get("Content-Type"))
		assert.Equal(t, "Failed to generate error response", string(mockResponseWriter.writtenData))
	})
}
