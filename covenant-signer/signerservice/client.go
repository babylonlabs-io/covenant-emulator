package signerservice

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/middlewares"
	"io"
	"net/http"
	"time"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/handlers"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/types"
	"github.com/btcsuite/btcd/btcec/v2"
)

const (
	// 1MB should be enough for the response
	maxResponseSize = 1 << 20 // 1MB
)

// addHMACHeader adds the HMAC header to the request if an HMAC key is provided
func addHMACHeader(req *http.Request, hmacKey string, body []byte) error {
	if hmacKey == "" {
		return nil
	}

	hmacValue, err := middlewares.GenerateHMAC(hmacKey, body)
	if err != nil {
		return fmt.Errorf("failed to generate HMAC: %w", err)
	}

	if hmacValue != "" {
		req.Header.Set(middlewares.HeaderCovenantHMAC, hmacValue)
	}

	return nil
}

func RequestCovenantSignaure(
	ctx context.Context,
	signerURL string,
	timeout time.Duration,
	preq *signerapp.ParsedSigningRequest,
	hmacKey string,
) (*signerapp.ParsedSigningResponse, error) {
	req, err := types.ToSignTransactionRequest(preq)

	if err != nil {
		return nil, err
	}

	marshalled, err := json.Marshal(req)

	if err != nil {
		return nil, err
	}

	route := fmt.Sprintf("%s/v1/sign-transactions", signerURL)

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, route, bytes.NewReader(marshalled))

	if err != nil {
		return nil, err
	}

	// use json
	httpRequest.Header.Set("Content-Type", "application/json")

	if err := addHMACHeader(httpRequest, hmacKey, marshalled); err != nil {
		return nil, err
	}

	client := http.Client{Timeout: timeout}
	// send the request
	res, err := client.Do(httpRequest)

	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	maxSizeReader := http.MaxBytesReader(nil, res.Body, maxResponseSize)

	// read body, up to 1MB
	resBody, err := io.ReadAll(maxSizeReader)

	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signing request failed. status code: %d, message: %s", res.StatusCode, string(resBody))
	}

	var response handlers.PublicResponse[types.SignTransactionsResponse]
	if err := json.Unmarshal(resBody, &response); err != nil {
		return nil, err
	}

	return types.ToParsedSigningResponse(&response.Data)
}

func GetPublicKey(ctx context.Context, signerURL string, timeout time.Duration, hmacKey string) (*btcec.PublicKey, error) {
	route := fmt.Sprintf("%s/v1/public-key", signerURL)

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, route, nil)

	if err != nil {
		return nil, err
	}

	if err := addHMACHeader(httpRequest, hmacKey, []byte{}); err != nil {
		return nil, err
	}

	client := http.Client{Timeout: timeout}
	res, err := client.Do(httpRequest)

	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	maxSizeReader := http.MaxBytesReader(nil, res.Body, maxResponseSize)

	resBody, err := io.ReadAll(maxSizeReader)

	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public key request failed. status code: %d, message: %s", res.StatusCode, string(resBody))
	}

	var response handlers.PublicResponse[types.GetPublicKeyResponse]
	if err := json.Unmarshal(resBody, &response); err != nil {
		return nil, err
	}

	pubKey, err := hex.DecodeString(response.Data.PublicKey)
	if err != nil {
		return nil, err
	}

	return btcec.ParsePubKey(pubKey)
}

func Unlock(ctx context.Context, signerURL string, timeout time.Duration, passphrase string, hmacKey string) error {
	route := fmt.Sprintf("%s/v1/unlock", signerURL)

	req := &types.UnlockRequest{
		Passphrase: passphrase,
	}
	marshalled, err := json.Marshal(req)

	if err != nil {
		return err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, route, bytes.NewReader(marshalled))

	if err != nil {
		return err
	}

	// use json
	httpRequest.Header.Set("Content-Type", "application/json")

	if err := addHMACHeader(httpRequest, hmacKey, marshalled); err != nil {
		return err
	}

	client := http.Client{Timeout: timeout}
	// send the request
	res, err := client.Do(httpRequest)

	if err != nil {
		return err
	}

	defer res.Body.Close()

	maxSizeReader := http.MaxBytesReader(nil, res.Body, maxResponseSize)

	// read body, up to 1MB
	resBody, err := io.ReadAll(maxSizeReader)

	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unlocking request failed. status code: %d, message: %s", res.StatusCode, string(resBody))
	}

	return nil
}

func Lock(ctx context.Context, signerURL string, timeout time.Duration, hmacKey string) error {
	route := fmt.Sprintf("%s/v1/lock", signerURL)
	var emptyBody []byte
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, route, bytes.NewReader(emptyBody))

	if err != nil {
		return err
	}

	// use json
	httpRequest.Header.Set("Content-Type", "application/json")

	if err := addHMACHeader(httpRequest, hmacKey, emptyBody); err != nil {
		return err
	}

	client := http.Client{Timeout: timeout}
	// send the request
	res, err := client.Do(httpRequest)

	if err != nil {
		return err
	}

	defer res.Body.Close()

	maxSizeReader := http.MaxBytesReader(nil, res.Body, maxResponseSize)

	// read body, up to 1MB
	resBody, err := io.ReadAll(maxSizeReader)

	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("locking request failed. status code: %d, message: %s", res.StatusCode, string(resBody))
	}

	return nil
}
