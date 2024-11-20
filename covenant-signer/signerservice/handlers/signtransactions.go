package handlers

import (
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
