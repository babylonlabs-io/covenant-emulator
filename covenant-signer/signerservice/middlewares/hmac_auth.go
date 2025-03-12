package middlewares

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/types"
	"io"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

const (
	// HeaderCovenantHMAC is the HTTP header name for the HMAC
	HeaderCovenantHMAC = "X-Covenant-HMAC"
)

// HMACAuthMiddleware creates a middleware that verifies HMAC authentication
func HMACAuthMiddleware(hmacKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip HMAC verification if no key is configured
			if hmacKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			receivedHMAC := r.Header.Get(HeaderCovenantHMAC)
			if receivedHMAC == "" {
				log.Debug().Msg("Request rejected: Missing HMAC header")
				RespondWithError(w, types.NewUnauthorizedError("missing HMAC authentication header"))
				return
			}

			body, newBody, err := RewindRequestBody(r.Body)
			if err != nil {
				log.Error().Err(err).Msg("Failed to read request body for HMAC verification")
				RespondWithError(w, types.NewInternalServiceError(err))
				return
			}

			r.Body = newBody

			valid, err := ValidateHMAC(hmacKey, body, receivedHMAC)
			if err != nil {
				log.Error().Err(err).Msg("Error validating HMAC")
				RespondWithError(w, types.NewInternalServiceError(err))
				return
			}

			if !valid {
				log.Debug().Msg("Request rejected: Invalid HMAC")
				RespondWithError(w, types.NewUnauthorizedError("invalid HMAC authentication"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GenerateHMAC generates an HMAC for a request body
func GenerateHMAC(hmacKey string, body []byte) (string, error) {
	if hmacKey == "" {
		return "", nil
	}

	h := hmac.New(sha256.New, []byte(hmacKey))
	_, err := h.Write(body)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ValidateHMAC validates the HMAC for a request
func ValidateHMAC(hmacKey string, body []byte, receivedHMAC string) (bool, error) {
	if hmacKey == "" {
		return true, nil
	}

	if receivedHMAC == "" {
		return false, nil
	}

	expectedHMAC, err := GenerateHMAC(hmacKey, body)
	if err != nil {
		return false, err
	}

	// Use constant-time comparison to prevent timing attacks
	return hmac.Equal([]byte(expectedHMAC), []byte(receivedHMAC)), nil
}

// RewindRequestBody reads a request body and then rewinds it, so it can be read again
func RewindRequestBody(reader io.ReadCloser) ([]byte, io.ReadCloser, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, err
	}

	return body, io.NopCloser(strings.NewReader(string(body))), nil
}

func RespondWithError(w http.ResponseWriter, appErr *types.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.StatusCode)

	response := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    appErr.ErrorCode.String(),
			"message": appErr.Error(),
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte("Failed to generate error response"))
		if err != nil {
			log.Error().Err(err).Msg("Failed to write error response")
		}
	}
}
