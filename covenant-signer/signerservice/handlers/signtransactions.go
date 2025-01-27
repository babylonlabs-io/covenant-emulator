package handlers

import (
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/types"
)

func (h *Handler) SignTransactions(request *http.Request) (*Result, *types.Error) {
	payload := &types.SignTransactionsRequest{}
	err := json.NewDecoder(request.Body).Decode(payload)
	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, "invalid request payload")
	}

	parsedRequest, err := types.ParseSigningRequest(payload)

	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, err.Error())
	}

	h.m.IncReceivedSigningRequests()

	sig, err := h.s.SignTransactions(
		request.Context(),
		parsedRequest,
	)

	if err != nil {
		h.m.IncFailedSigningRequests()

		// if this is unknown error, return internal server error
		return nil, types.NewErrorWithMsg(http.StatusInternalServerError, types.InternalServiceError, err.Error())
	}

	resp := types.ToResponse(sig)

	h.m.IncSuccessfulSigningRequests()

	return NewResult(resp), nil
}

func (h *Handler) GetPublicKey(request *http.Request) (*Result, *types.Error) {
	pubKey, err := h.s.PubKey(request.Context())
	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusInternalServerError, types.InternalServiceError, err.Error())
	}

	resp := &types.GetPublicKeyResponse{
		PublicKey: hex.EncodeToString(pubKey.SerializeCompressed()),
	}

	return NewResult(resp), nil
}

func (h *Handler) Unlock(request *http.Request) (*Result, *types.Error) {
	payload := &types.UnlockRequest{}
	err := json.NewDecoder(request.Body).Decode(payload)
	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, "invalid request payload")
	}

	err = h.s.Unlock(request.Context(), payload.Passphrase)
	if err != nil {
		h.m.IncFailedUnlockRequests()
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, err.Error())
	}

	h.m.SetSignerUnlocked()

	return NewResult(&types.UnlockResponse{}), nil
}

func (h *Handler) Lock(request *http.Request) (*Result, *types.Error) {
	err := h.s.Lock(request.Context())
	if err != nil {
		return nil, types.NewErrorWithMsg(http.StatusBadRequest, types.BadRequest, err.Error())
	}

	h.m.SetSignerLocked()

	return NewResult(&types.LockResponse{}), nil
}
