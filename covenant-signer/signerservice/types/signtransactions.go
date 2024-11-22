package types

import (
	"encoding/hex"
	"fmt"

	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
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
}

func ParseSigningRequest(request *SignTransactionsRequest) (*signerapp.ParsedSigningRequest, error) {
	stakingTx, _, err := utils.NewBTCTxFromHex(request.StakingTxHex)

	if err != nil {
		return nil, fmt.Errorf("invalid staking transaction in request: %w", err)
	}

	slashingTx, _, err := utils.NewBTCTxFromHex(request.SlashingTxHex)

	if err != nil {
		return nil, fmt.Errorf("invalid slashing transaction in request: %w", err)
	}

	unbondingTx, _, err := utils.NewBTCTxFromHex(request.UnbondingTxHex)

	if err != nil {
		return nil, fmt.Errorf("invalid unbonding transaction in request: %w", err)
	}

	slashUnbondingTx, _, err := utils.NewBTCTxFromHex(request.SlashUnbondingTxHex)

	if err != nil {
		return nil, fmt.Errorf("invalid slash unbonding transaction in request: %w", err)
	}

	slashingScript, err := hex.DecodeString(request.SlashingScriptHex)

	if err != nil {
		return nil, fmt.Errorf("invalid slashing script in request: %w", err)
	}

	if len(slashingScript) == 0 {
		return nil, fmt.Errorf("slashing script is empty")
	}

	unbondingScript, err := hex.DecodeString(request.UnbondingScriptHex)

	if err != nil {
		return nil, fmt.Errorf("invalid unbonding script in request: %w", err)
	}

	if len(unbondingScript) == 0 {
		return nil, fmt.Errorf("unbonding script is empty")
	}

	unbondingSlashingScript, err := hex.DecodeString(request.UnbondingSlashingScriptHex)

	if err != nil {
		return nil, fmt.Errorf("invalid unbonding slashing script in request: %w", err)
	}

	if len(unbondingSlashingScript) == 0 {
		return nil, fmt.Errorf("unbonding slashing script is empty")
	}

	fpEncKeys := make([]*asig.EncryptionKey, len(request.FpEncKeys))

	for i, key := range request.FpEncKeys {
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

	return &signerapp.ParsedSigningRequest{
		StakingTx:               stakingTx,
		SlashingTx:              slashingTx,
		UnbondingTx:             unbondingTx,
		SlashUnbondingTx:        slashUnbondingTx,
		StakingOutputIdx:        request.StakingOutputIdx,
		SlashingScript:          slashingScript,
		UnbondingScript:         unbondingScript,
		UnbondingSlashingScript: unbondingSlashingScript,
		FpEncKeys:               fpEncKeys,
	}, nil
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

	return &SignTransactionsRequest{
		StakingTxHex:               stakingTxHex,
		SlashingTxHex:              slashingTxHex,
		UnbondingTxHex:             unbondingTxHex,
		SlashUnbondingTxHex:        slashUnbondingTxHex,
		StakingOutputIdx:           parsedRequest.StakingOutputIdx,
		SlashingScriptHex:          hex.EncodeToString(parsedRequest.SlashingScript),
		UnbondingScriptHex:         hex.EncodeToString(parsedRequest.UnbondingScript),
		UnbondingSlashingScriptHex: hex.EncodeToString(parsedRequest.UnbondingSlashingScript),
		FpEncKeys:                  fpEncKeys,
	}, nil
}

type SignTransactionsResponse struct {
	SlashingTransactionsAdaptorSignatures       []string `json:"slashing_transactions_signatures"`
	UnbondingTransactionSignature               string   `json:"unbonding_transaction_signature"`
	SlashUnbondingTransactionsAdaptorSignatures []string `json:"slash_unbonding_transactions_signatures"`
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

	return &SignTransactionsResponse{
		SlashingTransactionsAdaptorSignatures:       slashAdaptorSigs,
		UnbondingTransactionSignature:               unbondingSig,
		SlashUnbondingTransactionsAdaptorSignatures: slashUnbondingAdaptorSigs,
	}
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

	return &signerapp.ParsedSigningResponse{
		SlashAdaptorSigs:          slashAdaptorSigs,
		UnbondingSig:              unbondingSig,
		SlashUnbondingAdaptorSigs: slashUnbondingAdaptorSigs,
	}, nil
}
