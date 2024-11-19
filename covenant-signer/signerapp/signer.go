package signerapp

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

var (
	ErrInvalidSigningRequest = fmt.Errorf("invalid signing request")
)

func wrapInvalidSigningRequestError(err error) error {
	return fmt.Errorf("%s: %w", err, ErrInvalidSigningRequest)
}

type SignerApp struct {
	s   ExternalBtcSigner
	r   BtcChainInfo
	p   BabylonParamsRetriever
	net *chaincfg.Params
}

func NewSignerApp(
	s ExternalBtcSigner,
	r BtcChainInfo,
	p BabylonParamsRetriever,
	net *chaincfg.Params,
) *SignerApp {
	return &SignerApp{
		s:   s,
		r:   r,
		p:   p,
		net: net,
	}
}

func (s *SignerApp) pubKeyToAddress(pubKey *btcec.PublicKey) (btcutil.Address, error) {
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	witnessAddr, err := btcutil.NewAddressWitnessPubKeyHash(
		pubKeyHash, s.net,
	)

	if err != nil {
		return nil, err
	}
	return witnessAddr, nil
}

func isCovenantMember(pubKey *btcec.PublicKey, covenantKeys []*btcec.PublicKey) bool {
	for _, key := range covenantKeys {
		if pubKey.IsEqual(key) {
			return true
		}
	}

	return false
}

func outputsAreEqual(a *wire.TxOut, b *wire.TxOut) bool {
	if a.Value != b.Value {
		return false
	}

	if !bytes.Equal(a.PkScript, b.PkScript) {
		return false
	}

	return true
}

// TODO: add unit tests for validations
func (s *SignerApp) SignUnbondingTransaction(
	ctx context.Context,
	stakingOutputPkScript []byte,
	unbondingTx *wire.MsgTx,
	stakerUnbondingSig *schnorr.Signature,
	covnentSignerPubKey *btcec.PublicKey,
) (*schnorr.Signature, error) {
	if err := btcstaking.CheckPreSignedUnbondingTxSanity(unbondingTx); err != nil {
		return nil, wrapInvalidSigningRequestError(err)
	}

	script, err := txscript.ParsePkScript(stakingOutputPkScript)

	if err != nil {
		return nil, wrapInvalidSigningRequestError(err)
	}

	if script.Class() != txscript.WitnessV1TaprootTy {
		return nil, wrapInvalidSigningRequestError(fmt.Errorf("invalid staking output pk script"))
	}

	stakingTxHash := unbondingTx.TxIn[0].PreviousOutPoint.Hash

	stakingTxInfo, err := s.r.TxByHash(ctx, &stakingTxHash, stakingOutputPkScript)

	if err != nil {
		return nil, err
	}
	bestBlock, err := s.r.BestBlockHeight(ctx)

	if err != nil {
		return nil, err
	}

	// TODO: This should probably be done when service is started, otherwise if we implement
	// retrieving params from service we will call it for every signing request
	params, err := s.p.ParamsByHeight(ctx, uint64(stakingTxInfo.TxInclusionHeight))

	if err != nil {
		return nil, err
	}

	if !isCovenantMember(covnentSignerPubKey, params.CovenantPublicKeys) {
		return nil, wrapInvalidSigningRequestError(fmt.Errorf("received covenant public key %s is not committee member at height %d",
			hex.EncodeToString(covnentSignerPubKey.SerializeCompressed()),
			stakingTxInfo.TxInclusionHeight,
		))
	}

	// We are using signed numbers here as calls to:
	// - TxByHash
	// - BestBlockHeight
	// are not atomic. This means if we do them during underlying node re-org
	// we may hit the case where stakingTxInfo.TxInclusionHeight is higher than bestBlock.
	numberOfStakingTxConfirmations := (int64(bestBlock) - int64(stakingTxInfo.TxInclusionHeight)) + 1

	if numberOfStakingTxConfirmations < int64(params.ConfirmationDepth) {
		return nil, wrapInvalidSigningRequestError(fmt.Errorf(
			"staking tx does not have enough confirmations. Current confirmations: %d, required confirmations: %d",
			numberOfStakingTxConfirmations,
			params.ConfirmationDepth,
		))
	}

	parsedStakingTransaction, err := btcstaking.ParseV0StakingTx(
		stakingTxInfo.Tx,
		params.MagicBytes,
		params.CovenantPublicKeys,
		params.CovenantQuorum,
		s.net)

	if err != nil {
		return nil, wrapInvalidSigningRequestError(err)
	}

	stakingOutputIndexFromUnbondingTx := unbondingTx.TxIn[0].PreviousOutPoint.Index

	//#nosec G115 -- safe conversion from int to uint32, as this point we know that
	// - staking transaction is valid BTC transaction that is part of the BTC ledger
	// - BTC transactions won't have more that math.MaxUint32 outputs (in reality the max is closer to ~4k output)
	if stakingOutputIndexFromUnbondingTx != uint32(parsedStakingTransaction.StakingOutputIdx) {
		return nil, wrapInvalidSigningRequestError(fmt.Errorf("unbonding transaction has invalid input index"))
	}

	if parsedStakingTransaction.OpReturnData.StakingTime < params.MinStakingTime ||
		parsedStakingTransaction.OpReturnData.StakingTime > params.MaxStakingTime {
		return nil, wrapInvalidSigningRequestError(
			fmt.Errorf(
				"staking time of staking tx with hash: %s is out of bounds",
				stakingTxHash.String(),
			),
		)
	}

	if parsedStakingTransaction.StakingOutput.Value < int64(params.MinStakingAmount) ||
		parsedStakingTransaction.StakingOutput.Value > int64(params.MaxStakingAmount) {
		return nil, wrapInvalidSigningRequestError(fmt.Errorf(
			"staking amount of staking tx with hash: %s is out of bounds",
			stakingTxHash.String(),
		))
	}

	expectedUnbondingOutputValue := parsedStakingTransaction.StakingOutput.Value - int64(params.UnbondingFee)

	if expectedUnbondingOutputValue <= 0 {
		// This is actually eror of our parameters configuaration and should not happen
		// for honest requests.
		return nil, fmt.Errorf("staking output value is too low")
	}

	// build expected output in unbonding transaction
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		parsedStakingTransaction.OpReturnData.StakerPublicKey.PubKey,
		[]*btcec.PublicKey{parsedStakingTransaction.OpReturnData.FinalityProviderPublicKey.PubKey},
		params.CovenantPublicKeys,
		params.CovenantQuorum,
		params.UnbondingTime,
		btcutil.Amount(expectedUnbondingOutputValue),
		s.net,
	)

	if err != nil {
		return nil, err
	}

	if !outputsAreEqual(unbondingInfo.UnbondingOutput, unbondingTx.TxOut[0]) {
		return nil, wrapInvalidSigningRequestError(
			fmt.Errorf("unbonding output does not match expected output"),
		)
	}

	// At this point we know that:
	// - unbonding tx has correct shape - 1 input, 1 output, no timelocks, not replaceable
	// - staking tx exists on btc chain, is mature and has correct shape according Babylong Params
	// - unbonding tx output matches the parameters from the staking transaction and the params
	// We can send request to our remote signer
	stakingInfo, err := btcstaking.BuildStakingInfo(
		parsedStakingTransaction.OpReturnData.StakerPublicKey.PubKey,
		[]*btcec.PublicKey{parsedStakingTransaction.OpReturnData.FinalityProviderPublicKey.PubKey},
		params.CovenantPublicKeys,
		params.CovenantQuorum,
		parsedStakingTransaction.OpReturnData.StakingTime,
		btcutil.Amount(parsedStakingTransaction.StakingOutput.Value),
		s.net,
	)

	if err != nil {
		return nil, err
	}

	unbondingPathInfo, err := stakingInfo.UnbondingPathSpendInfo()

	if err != nil {
		return nil, err
	}

	// Verify that staker signature is correct. This makes sure that this is staker
	// who requests unbonding or at least someone who has access to staker's private key
	err = btcstaking.VerifyTransactionSigWithOutput(
		unbondingTx,
		parsedStakingTransaction.StakingOutput,
		unbondingPathInfo.RevealedLeaf.Script,
		parsedStakingTransaction.OpReturnData.StakerPublicKey.PubKey,
		stakerUnbondingSig.Serialize(),
	)

	if err != nil {
		return nil, wrapInvalidSigningRequestError(
			fmt.Errorf(
				"staker unbonding signature verification failed: %w",
				err,
			),
		)
	}

	covenantKeyAddress, err := s.pubKeyToAddress(covnentSignerPubKey)

	if err != nil {
		return nil, err
	}

	sig, err := s.s.RawSignature(ctx, &SigningRequest{
		StakingOutput:        parsedStakingTransaction.StakingOutput,
		UnbondingTransaction: unbondingTx,
		CovenantPublicKey:    covnentSignerPubKey,
		CovenantAddress:      covenantKeyAddress,
		SpendDescription: &SpendPathDescription{
			ControlBlock: &unbondingPathInfo.ControlBlock,
			ScriptLeaf:   &unbondingPathInfo.RevealedLeaf,
		},
	})

	if err != nil {
		return nil, err
	}

	return sig.Signature, nil
}
