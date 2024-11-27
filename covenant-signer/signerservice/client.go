package signerservice

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

func RequestCovenantSignaure(
	ctx context.Context,
	signerUrl string,
	timeout time.Duration,
	preq *signerapp.ParsedSigningRequest,
) (*signerapp.ParsedSigningResponse, error) {

	req, err := types.ToSignTransactionRequest(preq)

	if err != nil {
		return nil, err
	}

	marshalled, err := json.Marshal(req)

	if err != nil {
		return nil, err
	}

	route := fmt.Sprintf("%s/v1/sign-transactions", signerUrl)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", route, bytes.NewReader(marshalled))

	if err != nil {
		return nil, err
	}

	// use json
	httpRequest.Header.Set("Content-Type", "application/json")

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

func GetPublicKey(ctx context.Context, signerUrl string, timeout time.Duration) (*btcec.PublicKey, error) {
	route := fmt.Sprintf("%s/v1/public-key", signerUrl)

	httpRequest, err := http.NewRequestWithContext(ctx, "GET", route, nil)

	if err != nil {
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

func Unlock(ctx context.Context, signerUrl string, timeout time.Duration, passphrase string) error {
	route := fmt.Sprintf("%s/v1/unlock", signerUrl)

	req := &types.UnlockRequest{
		Passphrase: passphrase,
	}
	marshalled, err := json.Marshal(req)

	if err != nil {
		return err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", route, bytes.NewReader(marshalled))

	if err != nil {
		return err
	}

	// use json
	httpRequest.Header.Set("Content-Type", "application/json")

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

func Lock(ctx context.Context, signerUrl string, timeout time.Duration) error {
	route := fmt.Sprintf("%s/v1/lock", signerUrl)
	httpRequest, err := http.NewRequestWithContext(ctx, "POST", route, bytes.NewReader([]byte{}))

	if err != nil {
		return err
	}

	// use json
	httpRequest.Header.Set("Content-Type", "application/json")

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
