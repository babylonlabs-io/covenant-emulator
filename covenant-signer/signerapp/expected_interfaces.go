package signerapp

import (
	"context"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

type BabylonParams struct {
	CovenantPublicKeys []*btcec.PublicKey
	CovenantQuorum     uint32
	MagicBytes         []byte
	UnbondingTime      uint16
	UnbondingFee       btcutil.Amount
	MaxStakingAmount   btcutil.Amount
	MinStakingAmount   btcutil.Amount
	MaxStakingTime     uint16
	MinStakingTime     uint16
	ConfirmationDepth  uint16
}

type BabylonParamsRetriever interface {
	// ParamsByHeight
	ParamsByHeight(ctx context.Context, height uint64) (*BabylonParams, error)
}

type TxInfo struct {
	Tx                *wire.MsgTx
	TxInclusionHeight uint32
}

type BtcChainInfo interface {
	// Returns only transactions inluded in canonical chain
	// passing pkScript as argument make it light client friendly
	TxByHash(ctx context.Context, txHash *chainhash.Hash, pkScript []byte) (*TxInfo, error)

	BestBlockHeight(ctx context.Context) (uint32, error)
}

type SpendPathDescription struct {
	ControlBlock *txscript.ControlBlock
	ScriptLeaf   *txscript.TapLeaf
}

type SigningRequest struct {
	StakingOutput        *wire.TxOut
	UnbondingTransaction *wire.MsgTx
	CovenantPublicKey    *btcec.PublicKey
	CovenantAddress      btcutil.Address
	SpendDescription     *SpendPathDescription
}

type SigningResult struct {
	Signature *schnorr.Signature
}

type ExternalBtcSigner interface {
	RawSignature(ctx context.Context, request *SigningRequest) (*SigningResult, error)
}
