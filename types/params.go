package types

import (
	sdkmath "cosmossdk.io/math"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
)

type StakingParams struct {
	// K-deep
	ComfirmationTimeBlocks uint64
	// W-deep
	FinalizationTimeoutBlocks uint64

	// Minimum amount of tx fee (quantified in Satoshi) needed for the pre-signed slashing tx
	MinSlashingTxFeeSat btcutil.Amount

	// Bitcoin public keys of the covenant committee
	CovenantPks []*btcec.PublicKey

	// PkScript that must be inserted in the slashing output of the slashing transaction
	SlashingPkScript []byte

	// Minimum number of signatures needed for the covenant multisignature
	CovenantQuorum uint32

	// The staked amount to be slashed, expressed as a decimal (e.g., 0.5 for 50%).
	SlashingRate sdkmath.LegacyDec

	// Minimum commission rate
	MinComissionRate sdkmath.LegacyDec

	// The minimum time for unbonding transaction timelock in BTC blocks
	MinUnbondingTime uint32

	// Fee required by unbonding transaction
	UnbondingFee btcutil.Amount

	// Minimum staking time required by babylon
	MinStakingTime uint16

	// Maximum staking time required by babylon
	MaxStakingTime uint16

	// Minimum staking value required by babylon
	MinStakingValue btcutil.Amount

	// Maximum staking value required by babylon
	MaxStakingValue btcutil.Amount
}

// MinimumUnbondingTime returns the minimum unbonding time. It is the bigger value from:
// - MinUnbondingTime
// - CheckpointFinalizationTimeout
func (p *StakingParams) MinimumUnbondingTime() uint64 {
	return sdkmath.Max[uint64](
		uint64(p.MinUnbondingTime),
		p.FinalizationTimeoutBlocks,
	)
}
