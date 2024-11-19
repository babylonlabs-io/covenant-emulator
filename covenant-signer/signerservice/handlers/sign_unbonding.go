package handlers

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/types"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/utils"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

func parseSchnorrSigFromHex(hexStr string) (*schnorr.Signature, error) {
	sigBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}

	return schnorr.ParseSignature(sigBytes)
}

func (h *Handler) SignUnbonding(request *http.Request) (*Result, *types.Error) {
	payload := &types.SignUnbondingTxRequest{}
	err := json.NewDecoder(request.Body).Decode(payload)
	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, "invalid request payload")
	}

	pkScript, err := hex.DecodeString(payload.StakingOutputPkScriptHex)

	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, "invalid staking output pk script")
	}

	covenantPublicKeyBytes, err := hex.DecodeString(payload.CovenantPublicKey)

	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, "invalid covenant public key")
	}

	covenantPublicKey, err := btcec.ParsePubKey(covenantPublicKeyBytes)

	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, "invalid covenant public key")
	}

	unbondingTx, _, err := utils.NewBTCTxFromHex(payload.UnbondingTxHex)

	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, "invalid unbonding transaction")
	}

	stakerUnbondingSig, err := parseSchnorrSigFromHex(payload.StakerUnbondingSigHex)

	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, "invalid staker unbonding signature")
	}

	// do not count the requests with invalid arguments
	h.m.IncReceivedSigningRequests()

	sig, err := h.s.SignUnbondingTransaction(
		request.Context(),
		pkScript,
		unbondingTx,
		stakerUnbondingSig,
		covenantPublicKey,
	)

	if err != nil {
		h.m.IncFailedSigningRequests()

		if errors.Is(err, signerapp.ErrInvalidSigningRequest) {
			return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, err.Error())
		}

		// if this is unknown error, return internal server error
		return nil, types.NewErrorWithMsg(http.StatusInternalServerError, types.InternalServiceError, err.Error())
	}

	resp := types.SignUnbondingTxResponse{
		SignatureHex: hex.EncodeToString(sig.Serialize()),
	}

	h.m.IncSuccessfulSigningRequests()

	return NewResult(resp), nil
}
