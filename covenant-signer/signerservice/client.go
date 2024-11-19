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

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/handlers"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/types"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/utils"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/wire"
)

const (
	// 1MB should be enough for the response
	maxResponseSize = 1 << 20 // 1MB
)

func RequestCovenantSignaure(
	ctx context.Context,
	signerUrl string,
	timeout time.Duration,
	unbondingTx *wire.MsgTx,
	stakerUnbondingSig *schnorr.Signature,
	covenantMemberPublicKey *btcec.PublicKey,
	stakingTransactionPkScript []byte,
) (*schnorr.Signature, error) {
	unbondingTxHex, err := utils.SerializeBTCTxToHex(unbondingTx)

	if err != nil {
		return nil, err
	}

	keyHex := hex.EncodeToString(covenantMemberPublicKey.SerializeCompressed())

	pkScriptHex := hex.EncodeToString(stakingTransactionPkScript)

	sigHex := hex.EncodeToString(stakerUnbondingSig.Serialize())

	req := types.SignUnbondingTxRequest{
		StakingOutputPkScriptHex: pkScriptHex,
		UnbondingTxHex:           unbondingTxHex,
		StakerUnbondingSigHex:    sigHex,
		CovenantPublicKey:        keyHex,
	}

	marshalled, err := json.Marshal(req)

	if err != nil {
		return nil, err
	}

	route := fmt.Sprintf("%s/v1/sign-unbonding-tx", signerUrl)

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

	var response handlers.PublicResponse[types.SignUnbondingTxResponse]
	if err := json.Unmarshal(resBody, &response); err != nil {
		return nil, err
	}

	return utils.SchnorSignatureFromHex(response.Data.SignatureHex)
}
