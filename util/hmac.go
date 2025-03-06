package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
)

const (
	// HeaderCovenantHMAC is the HTTP header name for the HMAC
	HeaderCovenantHMAC = "X-Covenant-HMAC"
)

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
