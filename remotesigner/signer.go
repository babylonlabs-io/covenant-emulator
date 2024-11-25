package remotesigner

import (
	"context"

	"github.com/babylonlabs-io/covenant-emulator/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice"
	"github.com/btcsuite/btcd/btcec/v2"
)

var _ covenant.Signer = RemoteSigner{}

func covenantRequestToSignerRequest(req covenant.SigningRequest) *signerapp.ParsedSigningRequest {
	return &signerapp.ParsedSigningRequest{
		StakingTx:               req.StakingTx,
		SlashingTx:              req.SlashingTx,
		UnbondingTx:             req.UnbondingTx,
		SlashUnbondingTx:        req.SlashUnbondingTx,
		StakingOutputIdx:        req.StakingOutputIdx,
		SlashingScript:          req.SlashingPkScriptPath,
		UnbondingScript:         req.StakingTxUnbondingPkScriptPath,
		UnbondingSlashingScript: req.UnbondingTxSlashingPkScriptPath,
		FpEncKeys:               req.FpEncKeys,
	}
}

func signerResponseToCovenantResponse(resp *signerapp.ParsedSigningResponse) *covenant.SignaturesResponse {
	return &covenant.SignaturesResponse{
		SlashSigs:          resp.SlashAdaptorSigs,
		UnbondingSig:       resp.UnbondingSig,
		SlashUnbondingSigs: resp.SlashUnbondingAdaptorSigs,
	}
}

type RemoteSigner struct {
	cfg *config.RemoteSignerCfg
}

func NewRemoteSigner(cfg *config.RemoteSignerCfg) RemoteSigner {
	return RemoteSigner{
		cfg: cfg,
	}
}

func (rs RemoteSigner) PubKey() (*btcec.PublicKey, error) {
	return signerservice.GetPublicKey(context.Background(), rs.cfg.URL, rs.cfg.Timeout)
}

func (rs RemoteSigner) SignTransactions(req covenant.SigningRequest) (*covenant.SignaturesResponse, error) {
	resp, err := signerservice.RequestCovenantSignaure(
		context.Background(),
		rs.cfg.URL,
		rs.cfg.Timeout,
		covenantRequestToSignerRequest(req),
	)

	if err != nil {
		return nil, err
	}

	return signerResponseToCovenantResponse(resp), nil
}
