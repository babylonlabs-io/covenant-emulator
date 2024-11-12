package covenant

import (
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/wire"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Signer wrapper interface to sign messages
type Signer interface {
	// SignTransactions signs all the transactions from the request
	// and returns all the signatures for Slash, Unbond and Unbonding Slash.
	SignTransactions(req SigningRequest) (*SignaturesResponse, error)
	// PubKey returns the current secp256k1 public key
	PubKey() (*secp.PublicKey, error)
}

type SigningRequest struct {
	StakingTx                       *wire.MsgTx
	SlashingTx                      *wire.MsgTx
	UnbondingTx                     *wire.MsgTx
	SlashUnbondingTx                *wire.MsgTx
	StakingOutputIdx                uint32
	SlashingPkScriptPath            []byte
	StakingTxUnbondingPkScriptPath  []byte
	UnbondingTxSlashingPkScriptPath []byte
	FpEncKeys                       []*asig.EncryptionKey
}

type SignaturesResponse struct {
	SlashSigs          [][]byte
	UnbondingSig       *schnorr.Signature
	SlashUnbondingSigs [][]byte
}
