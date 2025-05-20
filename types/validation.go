package types

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"

	"github.com/babylonlabs-io/babylon/btcstaking"
	bbn "github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/utils"
)

// ValidationResult contains the results of delegation validation
type ValidationResult struct {
	StakingOutputIdx   uint32
	UnbondingOutputIdx uint32
	StakingTx          *wire.MsgTx
	SlashingTx         *wire.MsgTx
	UnbondingTx        *wire.MsgTx
	SlashUnbondingTx   *wire.MsgTx
}

// ValidateDelegation validates a delegation against the staking parameters
func ValidateDelegation(
	delegation *Delegation,
	parameters *StakingParams,
	net *chaincfg.Params,
) (*ValidationResult, error) {
	// 1. Parse the staking and slashing transactions from hex
	stakingTx, _, err := utils.NewBTCTxFromHex(delegation.StakingTxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse staking tx hex: %w", err)
	}

	stakingSlashingTx, _, err := utils.NewBTCTxFromHex(delegation.SlashingTxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse slashing tx hex: %w", err)
	}

	// If we have an unbonding transaction, validate it
	var unbondingTx, unbondingSlashingTx *wire.MsgTx

	if delegation.BtcUndelegation != nil {
		unbondingTx, _, err = utils.NewBTCTxFromHex(delegation.BtcUndelegation.UnbondingTxHex)
		if err != nil {
			return nil, fmt.Errorf("failed to parse unbonding tx hex: %w", err)
		}

		unbondingSlashingTx, _, err = utils.NewBTCTxFromHex(delegation.BtcUndelegation.SlashingTxHex)
		if err != nil {
			return nil, fmt.Errorf("failed to parse unbonding slashing tx hex: %w", err)
		}
	}

	// 1. Validate unbonding time first as it will be used in other checks
	if uint32(delegation.UnbondingTime) != parameters.UnbondingTimeBlocks {
		return nil, fmt.Errorf("unbonding time %d must be equal to %d",
			delegation.UnbondingTime, parameters.UnbondingTimeBlocks)
	}

	stakingTxHash := stakingTx.TxHash()

	// 2. Validate all data related to staking tx:
	// - it has valid staking output
	// - that staking time and value are correct
	// - slashing tx is relevant to staking tx
	// - slashing tx signature is valid
	stakingInfo, err := btcstaking.BuildStakingInfo(
		delegation.BtcPk,
		delegation.FpBtcPks,
		parameters.CovenantPks,
		parameters.CovenantQuorum,
		uint16(delegation.StakingTime),
		delegation.TotalSat,
		net,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build staking info: %w", err)
	}

	stakingOutputIdx, err := bbn.GetOutputIdxInBTCTx(stakingTx, stakingInfo.StakingOutput)
	if err != nil {
		return nil, fmt.Errorf("staking tx does not contain expected staking output: %w", err)
	}

	if delegation.StakingTime < uint32(parameters.MinStakingTime) ||
		delegation.StakingTime > uint32(parameters.MaxStakingTime) {
		return nil, fmt.Errorf(
			"staking time %d is out of bounds. Min: %d, Max: %d",
			delegation.StakingTime,
			parameters.MinStakingTime,
			parameters.MaxStakingTime,
		)
	}

	if stakingTx.TxOut[stakingOutputIdx].Value < int64(parameters.MinStakingValue) ||
		stakingTx.TxOut[stakingOutputIdx].Value > int64(parameters.MaxStakingValue) {
		return nil, fmt.Errorf(
			"staking value %d is out of bounds. Min: %d, Max: %d",
			stakingTx.TxOut[stakingOutputIdx].Value,
			parameters.MinStakingValue,
			parameters.MaxStakingValue,
		)
	}

	if err := btcstaking.CheckSlashingTxMatchFundingTx(
		stakingSlashingTx,
		stakingTx,
		stakingOutputIdx,
		int64(parameters.MinSlashingTxFeeSat),
		parameters.SlashingRate,
		parameters.SlashingPkScript,
		delegation.BtcPk,
		delegation.UnbondingTime,
		net,
	); err != nil {
		return nil, fmt.Errorf("invalid staking tx: %w", err)
	}

	// If we have an unbonding tx, validate it
	var unbondingOutputIdx uint32 = 0
	if delegation.BtcUndelegation != nil {
		// 3. Validate all data related to unbonding tx:
		// - it is valid BTC pre-signed transaction
		// - it has valid unbonding output
		// - slashing tx is relevant to unbonding tx
		// - slashing tx signature is valid
		if err := btcstaking.CheckPreSignedUnbondingTxSanity(
			unbondingTx,
		); err != nil {
			return nil, fmt.Errorf("unbonding tx is not a valid pre-signed transaction: %w", err)
		}

		unbondingInfo, err := btcstaking.BuildUnbondingInfo(
			delegation.BtcPk,
			delegation.FpBtcPks,
			parameters.CovenantPks,
			parameters.CovenantQuorum,
			delegation.UnbondingTime,
			delegation.TotalSat-parameters.UnbondingFee,
			net,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to build the unbonding info: %w", err)
		}

		if !bytes.Equal(unbondingTx.TxOut[0].PkScript, unbondingInfo.UnbondingOutput.PkScript) {
			return nil, fmt.Errorf("the unbonding output script is not expected, expected: %x, got: %x",
				unbondingInfo.UnbondingOutput.PkScript, unbondingTx.TxOut[0].PkScript)
		}

		if unbondingTx.TxOut[0].Value != unbondingInfo.UnbondingOutput.Value {
			return nil, fmt.Errorf("the unbonding output value is not expected, expected: %d, got: %d",
				unbondingInfo.UnbondingOutput.Value, unbondingTx.TxOut[0].Value)
		}

		err = btcstaking.CheckSlashingTxMatchFundingTx(
			unbondingSlashingTx,
			unbondingTx,
			0, // unbonding output always has only 1 output
			int64(parameters.MinSlashingTxFeeSat),
			parameters.SlashingRate,
			parameters.SlashingPkScript,
			delegation.BtcPk,
			delegation.UnbondingTime,
			net,
		)
		if err != nil {
			return nil, fmt.Errorf("invalid unbonding tx: %w", err)
		}

		// 4. Check that unbonding tx input is pointing to staking tx
		if !unbondingTx.TxIn[0].PreviousOutPoint.Hash.IsEqual(&stakingTxHash) {
			return nil, fmt.Errorf("unbonding transaction must spend staking output")
		}

		if unbondingTx.TxIn[0].PreviousOutPoint.Index != stakingOutputIdx {
			return nil, fmt.Errorf("unbonding transaction input must spend staking output")
		}

		// 5. Check unbonding tx fees against staking tx.
		// - fee is larger than 0
		if unbondingTx.TxOut[0].Value >= stakingTx.TxOut[stakingOutputIdx].Value {
			return nil, fmt.Errorf("unbonding tx fee must be larger than 0")
		}

		// 6. Check that unbonding tx fee is as expected.
		unbondingTxFee := stakingTx.TxOut[stakingOutputIdx].Value - unbondingTx.TxOut[0].Value

		if unbondingTxFee != int64(parameters.UnbondingFee) {
			return nil, fmt.Errorf("unbonding tx fee must be %d, but got %d",
				parameters.UnbondingFee, unbondingTxFee)
		}
	}

	return &ValidationResult{
		StakingOutputIdx:   stakingOutputIdx,
		UnbondingOutputIdx: unbondingOutputIdx,
		StakingTx:          stakingTx,
		SlashingTx:         stakingSlashingTx,
		UnbondingTx:        unbondingTx,
		SlashUnbondingTx:   unbondingSlashingTx,
	}, nil
}
