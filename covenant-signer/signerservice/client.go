package signerservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/handlers"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/types"
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
