package types

import (
	"encoding/hex"
	"fmt"

	"github.com/babylonlabs-io/babylon/v3/btcstaking"
	asig "github.com/babylonlabs-io/babylon/v3/crypto/schnorr-adaptor-signature"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/utils"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

type SignTransactionsRequest struct {
	StakingTxHex               string   `json:"staking_tx_hex"`
	SlashingTxHex              string   `json:"slashing_tx_hex"`
	UnbondingTxHex             string   `json:"unbonding_tx_hex"`
	SlashUnbondingTxHex        string   `json:"slash_unbonding_tx_hex"`
	StakingOutputIdx           uint32   `json:"staking_output_idx"`
	SlashingScriptHex          string   `json:"slashing_script_hex"`
	UnbondingScriptHex         string   `json:"unbonding_script_hex"`
	UnbondingSlashingScriptHex string   `json:"unbonding_slashing_script_hex"`
	FpEncKeys                  []string `json:"fp_enc_keys"`
	// Stake Expansion Fields

	// PreviousActiveStakeTxHex the hex of the entire previous active BTC delegation.
	PreviousActiveStakeTxHex string `json:"previous_active_stake_tx_hex"`
	// OtherFundingOutputHex the hex of the wire.TxOut that is the other funding
	// output that will pay for the fees and possibly increase the amount staked.
	OtherFundingOutputHex string `json:"other_funding_output_hex"`
	// PrevStakingOutputIdx the index of the previous active stake output in the
	// previous active stake transaction. This is used to sign the spend of the
	// previous active stake transaction into BTC delegation expansion.
	PrevStakingOutputIdx uint32 `json:"previous_staking_output_idx"`
	// PreviousActiveStakeUnbondingScriptHex the hex of the unbonding script of the
	// previous active stake transaction. This is used to sign the spend of the
	// previous active stake transaction into BTC delegation expansion.
	PreviousActiveStakeUnbondingScriptHex string `json:"previous_active_stake_unbonding_script_hex"`
}

func ParseSigningRequest(req *SignTransactionsRequest) (*signerapp.ParsedSigningRequest, error) {
	stakingTx, _, err := utils.NewBTCTxFromHex(req.StakingTxHex)

	if err != nil {
		return nil, fmt.Errorf("invalid staking transaction in request: %w", err)
	}

	slashingTx, _, err := utils.NewBTCTxFromHex(req.SlashingTxHex)

	if err != nil {
		return nil, fmt.Errorf("invalid slashing transaction in request: %w", err)
	}

	unbondingTx, _, err := utils.NewBTCTxFromHex(req.UnbondingTxHex)

	if err != nil {
		return nil, fmt.Errorf("invalid unbonding transaction in request: %w", err)
	}

	slashUnbondingTx, _, err := utils.NewBTCTxFromHex(req.SlashUnbondingTxHex)

	if err != nil {
		return nil, fmt.Errorf("invalid slash unbonding transaction in request: %w", err)
	}

	slashingScript, err := hex.DecodeString(req.SlashingScriptHex)

	if err != nil {
		return nil, fmt.Errorf("invalid slashing script in request: %w", err)
	}

	if len(slashingScript) == 0 {
		return nil, fmt.Errorf("slashing script is empty")
	}

	unbondingScript, err := hex.DecodeString(req.UnbondingScriptHex)

	if err != nil {
		return nil, fmt.Errorf("invalid unbonding script in request: %w", err)
	}

	if len(unbondingScript) == 0 {
		return nil, fmt.Errorf("unbonding script is empty")
	}

	unbondingSlashingScript, err := hex.DecodeString(req.UnbondingSlashingScriptHex)

	if err != nil {
		return nil, fmt.Errorf("invalid unbonding slashing script in request: %w", err)
	}

	if len(unbondingSlashingScript) == 0 {
		return nil, fmt.Errorf("unbonding slashing script is empty")
	}

	fpEncKeys := make([]*asig.EncryptionKey, len(req.FpEncKeys))

	for i, key := range req.FpEncKeys {
		encKeyBytes, err := hex.DecodeString(key)

		if err != nil {
			return nil, fmt.Errorf("invalid fp encryption key in request: %w", err)
		}

		fpEncKey, err := asig.NewEncryptionKeyFromBytes(encKeyBytes)

		if err != nil {
			return nil, fmt.Errorf("invalid fp encryption key in request: %w", err)
		}

		fpEncKeys[i] = fpEncKey
	}

	parsedReq := &signerapp.ParsedSigningRequest{
		StakingTx:               stakingTx,
		SlashingTx:              slashingTx,
		UnbondingTx:             unbondingTx,
		SlashUnbondingTx:        slashUnbondingTx,
		StakingOutputIdx:        req.StakingOutputIdx,
		SlashingScript:          slashingScript,
		UnbondingScript:         unbondingScript,
		UnbondingSlashingScript: unbondingSlashingScript,
		FpEncKeys:               fpEncKeys,
		StakeExp:                nil,
	}

	if len(req.PreviousActiveStakeTxHex) > 0 {
		previousActiveStakeTx, _, err := utils.NewBTCTxFromHex(req.PreviousActiveStakeTxHex)
		if err != nil {
			return nil, fmt.Errorf("invalid previous active staking transaction %s in request: %w", req.PreviousActiveStakeTxHex, err)
		}

		otherFundingTxOutBz, err := hex.DecodeString(req.OtherFundingOutputHex)
		if err != nil {
			return nil, fmt.Errorf("invalid other funding output hex %s in request: %w", req.OtherFundingOutputHex, err)
		}

		otherFundingTxOut, err := btcstaking.DeserializeTxOut(otherFundingTxOutBz)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize other funding output from hex %s in request: %w", req.OtherFundingOutputHex, err)
		}

		prevStakeUnbondScriptBz, err := hex.DecodeString(req.PreviousActiveStakeUnbondingScriptHex)
		if err != nil {
			return nil, fmt.Errorf("invalid previous active stake unbonding script hex %s in request: %w", req.PreviousActiveStakeUnbondingScriptHex, err)
		}

		parsedReq.StakeExp = &signerapp.ParsedSigningRequestStkExp{
			PreviousActiveStakeTx:              previousActiveStakeTx,
			OtherFundingOutput:                 otherFundingTxOut,
			PreviousStakingOutputIdx:           req.PrevStakingOutputIdx,
			PreviousActiveStakeUnbondingScript: prevStakeUnbondScriptBz,
		}
	}

	return parsedReq, nil
}

func ToSignTransactionRequest(parsedRequest *signerapp.ParsedSigningRequest) (*SignTransactionsRequest, error) {
	stakingTxHex, err := utils.SerializeBTCTxToHex(parsedRequest.StakingTx)

	if err != nil {
		return nil, fmt.Errorf("failed to serialize staking transaction: %w", err)
	}

	slashingTxHex, err := utils.SerializeBTCTxToHex(parsedRequest.SlashingTx)

	if err != nil {
		return nil, fmt.Errorf("failed to serialize slashing transaction: %w", err)
	}

	unbondingTxHex, err := utils.SerializeBTCTxToHex(parsedRequest.UnbondingTx)

	if err != nil {
		return nil, fmt.Errorf("failed to serialize unbonding transaction: %w", err)
	}

	slashUnbondingTxHex, err := utils.SerializeBTCTxToHex(parsedRequest.SlashUnbondingTx)

	if err != nil {
		return nil, fmt.Errorf("failed to serialize slash unbonding transaction: %w", err)
	}

	fpEncKeys := make([]string, len(parsedRequest.FpEncKeys))

	for i, key := range parsedRequest.FpEncKeys {
		fpEncKeys[i] = hex.EncodeToString(key.ToBytes())
	}

	req := &SignTransactionsRequest{
		StakingTxHex:               stakingTxHex,
		SlashingTxHex:              slashingTxHex,
		UnbondingTxHex:             unbondingTxHex,
		SlashUnbondingTxHex:        slashUnbondingTxHex,
		StakingOutputIdx:           parsedRequest.StakingOutputIdx,
		SlashingScriptHex:          hex.EncodeToString(parsedRequest.SlashingScript),
		UnbondingScriptHex:         hex.EncodeToString(parsedRequest.UnbondingScript),
		UnbondingSlashingScriptHex: hex.EncodeToString(parsedRequest.UnbondingSlashingScript),
		FpEncKeys:                  fpEncKeys,
	}

	if parsedRequest.StakeExp != nil {
		pasTxHex, err := utils.SerializeBTCTxToHex(parsedRequest.StakeExp.PreviousActiveStakeTx)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize previous active stake transaction: %w", err)
		}
		ofoBz, err := btcstaking.SerializeTxOut(parsedRequest.StakeExp.OtherFundingOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize other funding output: %w", err)
		}

		req.PreviousActiveStakeTxHex = pasTxHex
		req.OtherFundingOutputHex = hex.EncodeToString(ofoBz)
		req.PrevStakingOutputIdx = parsedRequest.StakeExp.PreviousStakingOutputIdx
		req.PreviousActiveStakeUnbondingScriptHex = hex.EncodeToString(parsedRequest.StakeExp.PreviousActiveStakeUnbondingScript)
	}

	return req, nil
}

type SignTransactionsResponse struct {
	SlashingTransactionsAdaptorSignatures       []string `json:"slashing_transactions_signatures"`
	UnbondingTransactionSignature               string   `json:"unbonding_transaction_signature"`
	SlashUnbondingTransactionsAdaptorSignatures []string `json:"slash_unbonding_transactions_signatures"`
	// StakeExpansionTransactionSignature is the signature to allow the spent of the previous active
	// staking transaction into a new BTC delegation
	StakeExpansionTransactionSignature string `json:"stake_expansion_transaction_signature"`
}

func ToResponse(response *signerapp.ParsedSigningResponse) *SignTransactionsResponse {
	slashAdaptorSigs := make([]string, len(response.SlashAdaptorSigs))

	for i, sig := range response.SlashAdaptorSigs {
		slashAdaptorSigs[i] = hex.EncodeToString(sig)
	}

	unbondingSig := hex.EncodeToString(response.UnbondingSig.Serialize())

	slashUnbondingAdaptorSigs := make([]string, len(response.SlashUnbondingAdaptorSigs))

	for i, sig := range response.SlashUnbondingAdaptorSigs {
		slashUnbondingAdaptorSigs[i] = hex.EncodeToString(sig)
	}

	resp := &SignTransactionsResponse{
		SlashingTransactionsAdaptorSignatures:       slashAdaptorSigs,
		UnbondingTransactionSignature:               unbondingSig,
		SlashUnbondingTransactionsAdaptorSignatures: slashUnbondingAdaptorSigs,
	}

	if response.StakeExpSig != nil {
		stakeExpSig := hex.EncodeToString(response.StakeExpSig.Serialize())
		resp.StakeExpansionTransactionSignature = stakeExpSig
	}

	return resp
}

func ToParsedSigningResponse(response *SignTransactionsResponse) (*signerapp.ParsedSigningResponse, error) {
	if len(response.SlashingTransactionsAdaptorSignatures) == 0 {
		return nil, fmt.Errorf("no slashing transactions adaptor signatures in response")
	}

	if len(response.SlashUnbondingTransactionsAdaptorSignatures) == 0 {
		return nil, fmt.Errorf("no slash unbonding transactions adaptor signatures in response")
	}

	slashAdaptorSigs := make([][]byte, len(response.SlashingTransactionsAdaptorSignatures))

	for i, sig := range response.SlashingTransactionsAdaptorSignatures {
		adaptorSigBytes, err := hex.DecodeString(sig)

		if err != nil {
			return nil, fmt.Errorf("invalid slashing transactions adaptor signature in response: %w", err)
		}

		slashAdaptorSigs[i] = adaptorSigBytes
	}

	unbondingSigBytes, err := hex.DecodeString(response.UnbondingTransactionSignature)

	if err != nil {
		return nil, fmt.Errorf("invalid unbonding transaction signature in response: %w", err)
	}

	unbondingSig, err := schnorr.ParseSignature(unbondingSigBytes)

	if err != nil {
		return nil, fmt.Errorf("invalid unbonding transaction signature in response: %w", err)
	}

	slashUnbondingAdaptorSigs := make([][]byte, len(response.SlashUnbondingTransactionsAdaptorSignatures))

	for i, sig := range response.SlashUnbondingTransactionsAdaptorSignatures {
		adaptorSigBytes, err := hex.DecodeString(sig)

		if err != nil {
			return nil, fmt.Errorf("invalid slash unbonding transactions adaptor signature in response: %w", err)
		}

		slashUnbondingAdaptorSigs[i] = adaptorSigBytes
	}

	resp := &signerapp.ParsedSigningResponse{
		SlashAdaptorSigs:          slashAdaptorSigs,
		UnbondingSig:              unbondingSig,
		SlashUnbondingAdaptorSigs: slashUnbondingAdaptorSigs,
		StakeExpSig:               nil,
	}

	if len(response.StakeExpansionTransactionSignature) > 0 {
		stakeExpSigBytes, err := hex.DecodeString(response.StakeExpansionTransactionSignature)
		if err != nil {
			return nil, fmt.Errorf("invalid stake expansion signature %s in response: %w", response.StakeExpansionTransactionSignature, err)
		}

		stakeExpSig, err := schnorr.ParseSignature(stakeExpSigBytes)
		if err != nil {
			return nil, fmt.Errorf("invalid parse of stake expansion signature %s in response: %w", response.StakeExpansionTransactionSignature, err)
		}

		resp.StakeExpSig = stakeExpSig
	}

	return resp, nil
}
