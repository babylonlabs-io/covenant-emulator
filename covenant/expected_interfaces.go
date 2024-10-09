package covenant

import (
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/wire"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// CriptoSigner wrapper interface to sign messages
type CriptoSigner interface {
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
