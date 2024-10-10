package keyring

import (
	"fmt"
	"strings"

	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/covenant-emulator/covenant"

	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/wire"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

var _ covenant.Signer = KeyringSigner{}

type KeyringSigner struct {
	kc         *ChainKeyringController
	passphrase string
}

func NewKeyringSigner(chainId, keyName, keyringDir, keyringBackend, passphrase string) (*KeyringSigner, error) {
	input := strings.NewReader("")
	kr, err := CreateKeyring(keyringDir, chainId, keyringBackend, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	kc, err := NewChainKeyringControllerWithKeyring(kr, keyName, input)
	if err != nil {
		return nil, err
	}

	return &KeyringSigner{
		kc:         kc,
		passphrase: passphrase,
	}, nil
}

func (kcs KeyringSigner) PubKey() (*secp.PublicKey, error) {
	record, err := kcs.kc.KeyRecord()
	if err != nil {
		return nil, err
	}

	pubKey, err := record.GetPubKey()
	if err != nil {
		return nil, err
	}

	return btcec.ParsePubKey(pubKey.Bytes())
}

// getPrivKey returns the keyring private key
// TODO: update btcstaking functions to avoid receiving private key as parameter
// and only sign it using the kcs.kc.GetKeyring().Sign()
func (kcs KeyringSigner) getPrivKey() (*btcec.PrivateKey, error) {
	sdkPrivKey, err := kcs.kc.GetChainPrivKey(kcs.passphrase)
	if err != nil {
		return nil, err
	}

	privKey, _ := btcec.PrivKeyFromBytes(sdkPrivKey.Key)
	return privKey, nil
}

// EncSignTxWithOneScriptSpendInputStrict is encrypted version of
// SignTxWithOneScriptSpendInputStrict with the output to be encrypted
// by an encryption key (adaptor signature)
func (kcs KeyringSigner) EncSignTxWithOneScriptSpendInputStrict(
	txToSign *wire.MsgTx,
	fundingTx *wire.MsgTx,
	fundingOutputIdx uint32,
	signedScriptPath []byte,
	encKey *asig.EncryptionKey,
) (*asig.AdaptorSignature, error) {
	covenantPrivKey, err := kcs.getPrivKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get Covenant private key: %w", err)
	}

	return btcstaking.EncSignTxWithOneScriptSpendInputStrict(
		txToSign,
		fundingTx,
		fundingOutputIdx,
		signedScriptPath,
		covenantPrivKey,
		encKey,
	)
}

// SignTxWithOneScriptSpendInputStrict signs transaction with one input coming
// from script spend output with provided script.
// It checks:
// - txToSign is not nil
// - txToSign has exactly one input
// - fundingTx is not nil
// - fundingTx has one output committing to the provided script
// - txToSign input is pointing to the correct output in fundingTx
func (kcs KeyringSigner) SignTxWithOneScriptSpendInputStrict(
	txToSign *wire.MsgTx,
	fundingTx *wire.MsgTx,
	fundingOutputIdx uint32,
	signedScriptPath []byte,
) (*schnorr.Signature, error) {
	covenantPrivKey, err := kcs.getPrivKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get Covenant private key: %w", err)
	}

	return btcstaking.SignTxWithOneScriptSpendInputStrict(
		txToSign,
		fundingTx,
		fundingOutputIdx,
		signedScriptPath,
		covenantPrivKey,
	)
}
