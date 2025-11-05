//nolint:revive
package types

import (
	"encoding/hex"
	"math"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"

	"github.com/babylonlabs-io/babylon/v4/btcstaking"
	bbn "github.com/babylonlabs-io/babylon/v4/types"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

type Delegation struct {
	// The Bitcoin secp256k1 PK of this BTC delegation
	BtcPk *btcec.PublicKey
	// The Bitcoin secp256k1 PKs of the finality providers that
	// this BTC delegation delegates to
	FpBtcPks []*btcec.PublicKey
	// The number of blocks for which the delegation is locked on BTC chain
	StakingTime uint32
	// The start BTC height of the BTC delegation
	// it is the start BTC height of the timelock
	StartHeight uint32
	// The end height of the BTC delegation
	// it is the end BTC height of the timelock - w
	EndHeight uint32
	// The total amount of BTC stakes in this delegation
	// quantified in satoshi
	TotalSat btcutil.Amount
	// The hex string of the staking tx
	StakingTxHex string
	// The index of the staking output in the staking tx
	StakingOutputIdx uint32
	// The hex string of the slashing tx
	SlashingTxHex string
	// UnbondingTime describes how long the funds will be locked either in unbonding output
	// or slashing change output
	UnbondingTime uint16
	// The signatures on the slashing tx
	// by the covenants (i.e., SKs corresponding to covenant_pks in params)
	// It will be a part of the witness for the staking tx output.
	CovenantSigs []*CovenantAdaptorSigInfo
	// if this object is present it means that staker requested undelegation, and whole
	// delegation is being undelegated directly in delegation object
	BtcUndelegation *Undelegation
	// params version used to validate delegation
	ParamsVersion uint32
	// StakeExpansion is the hash of the staking tx that was used as
	// input to the stake expansion, if empty it is NOT a stake expansion.
	StakeExpansion *DelegationStakeExpansion
	// MultisigInfo contains additional staker info that is used for multisig
	// btc delegation, if empty it is NOT a multisig btc delegation.
	MultisigInfo *AdditionalStakerInfo
}

// AdditionalStakerInfo necessary info to construct covenant signatures for
// multisig btc delegation
type AdditionalStakerInfo struct {
	// StakerBtcPkList is the list of btc staker's pk and the main btc pk is not
	// included in this list. i.e., for M-of-N multisig, the length of StakerBtcPkList is N-1
	StakerBtcPkList []*btcec.PublicKey
	// StakerQuorum is a threshold of M-of-N multisig
	StakerQuorum uint32
}

// DelegationStakeExpansion necessary info to construct covenant signatures
// for the stake expansion
type DelegationStakeExpansion struct {
	// PreviousStakingTxHashHex is the hex hash of the staking tx that was used as
	// input to the stake expansion.
	PreviousStakingTxHashHex string
	// OtherFundingTxOutHex is the other funding output (TxOut) which was used
	// as input to construct the BTC delegation. The stake expansion has a set of
	// 2 inputs, the first input is the previous staking transaction and the
	// second input (this one) is to pay for fees and optionally to add more
	// stake to the BTC delegation.
	OtherFundingTxOutHex string
}

func (d *DelegationStakeExpansion) OtherFundingTxOut() (*wire.TxOut, error) {
	otherFundingTxOut, err := hex.DecodeString(d.OtherFundingTxOutHex)
	if err != nil {
		return nil, err
	}

	return btcstaking.DeserializeTxOut(otherFundingTxOut)
}

// HasCovenantQuorum returns whether a delegation has sufficient sigs
// from Covenant members to make a quorum
func (d *Delegation) HasCovenantQuorum(quorum uint32) bool {
	return len(d.CovenantSigs) >= int(quorum) && d.BtcUndelegation.HasAllSignatures(quorum)
}

func (d *Delegation) GetStakingTime() uint16 {
	if d.StakingTime > math.MaxUint16 {
		// In a valid delegation, EndHeight is always greater than StartHeight and it is always uint16 value
		panic("invalid delegation in database")
	}

	return uint16(d.StakingTime)
}

func (d *Delegation) IsStakeExpansion() bool {
	return d.StakeExpansion != nil
}

func (d *Delegation) IsMultisigBtcDel() bool {
	return d.MultisigInfo != nil
}

// Undelegation signalizes that the delegation is being undelegated
type Undelegation struct {
	// The hex string of the transaction which will transfer the funds from staking
	// output to unbonding output. Unbonding output will usually have lower timelock
	// than staking output.
	UnbondingTxHex string
	// The hex string of the slashing tx for unbonding transactions
	// It is partially signed by SK corresponding to btc_pk, but not signed by
	// finality provider or covenant yet.
	SlashingTxHex string
	// The signatures on the slashing tx by the covenant
	// (i.e., SK corresponding to covenant_pk in params)
	// It must be provided after processing undelagate message by the consumer chain
	CovenantSlashingSigs []*CovenantAdaptorSigInfo
	// The signatures on the unbonding tx by the covenant
	// (i.e., SK corresponding to covenant_pk in params)
	// It must be provided after processing undelagate message by the consumer chain
	CovenantUnbondingSigs []*CovenantSchnorrSigInfo
	// The delegator signature for the unbonding tx
	DelegatorUnbondingSig *bbn.BIP340Signature
	// The transaction that spends the staking tx output but not unbonding tx
	SpendStakeTxHex string
}

func (ud *Undelegation) HasCovenantQuorumOnSlashing(quorum uint32) bool {
	return len(ud.CovenantUnbondingSigs) >= int(quorum)
}

func (ud *Undelegation) HasCovenantQuorumOnUnbonding(quorum uint32) bool {
	return len(ud.CovenantUnbondingSigs) >= int(quorum)
}

func (ud *Undelegation) HasAllSignatures(covenantQuorum uint32) bool {
	return ud.HasCovenantQuorumOnUnbonding(covenantQuorum) && ud.HasCovenantQuorumOnSlashing(covenantQuorum)
}

type CovenantAdaptorSigInfo struct {
	Pk   *btcec.PublicKey
	Sigs [][]byte
}

type CovenantSchnorrSigInfo struct {
	Pk  *btcec.PublicKey
	Sig *schnorr.Signature
}
