package covenant

import (
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

type SigningTxsRequest struct {
	StakingTx                      *wire.MsgTx
	SlashingTx                     *wire.MsgTx
	UnbondingTx                    *wire.MsgTx
	SlashUnbondingTx               *wire.MsgTx
	StakingOutputIdx               uint32
	SlashingPkScriptPath           []byte
	StakingTxUnbondingPkScriptPath []byte
	UnbondingTxSlashingScriptPath  []byte
	FpEncKeys                      []*asig.EncryptionKey
}

type SigningRequest struct {
	SigningTxsReqByStkTxHash map[chainhash.Hash]SigningTxsRequest
}

type SignaturesResponse struct {
	SlashSigs          [][]byte
	UnbondingSig       *schnorr.Signature
	SlashUnbondingSigs [][]byte
}

type SigningResponse struct {
	SignaturesByStkTxHash map[chainhash.Hash]SignaturesResponse
}

// Signer wrapper interface to sign messages
type Signer interface {
	SignTransactions(req SigningRequest) (*SigningResponse, error)
	// PubKey returns the current secp256k1 public key
	PubKey() (*secp.PublicKey, error)
	// EncSignTxWithOneScriptSpendInputStrict is encrypted version of
	// SignTxWithOneScriptSpendInputStrict with the output to be encrypted
	// by an encryption key (adaptor signature)
	EncSignTxWithOneScriptSpendInputStrict(
		txToSign *wire.MsgTx,
		fundingTx *wire.MsgTx,
		fundingOutputIdx uint32,
		signedScriptPath []byte,
		encKey *asig.EncryptionKey,
	) (*asig.AdaptorSignature, error)
	// SignTxWithOneScriptSpendInputStrict signs transaction with one input coming
	// from script spend output with provided script.
	// It checks:
	// - txToSign is not nil
	// - txToSign has exactly one input
	// - fundingTx is not nil
	// - fundingTx has one output committing to the provided script
	// - txToSign input is pointing to the correct output in fundingTx
	SignTxWithOneScriptSpendInputStrict(
		txToSign *wire.MsgTx,
		fundingTx *wire.MsgTx,
		fundingOutputIdx uint32,
		signedScriptPath []byte,
	) (*schnorr.Signature, error)
}
