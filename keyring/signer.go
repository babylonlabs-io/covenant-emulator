package keyring

import (
	"fmt"
	"strings"

	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/covenant-emulator/covenant"

	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
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

// SignTransactions receives BTC delegation transactions to sign and returns all the signatures needed if nothing fails.
func (kcs KeyringSigner) SignTransactions(req covenant.SigningRequest) (*covenant.SignaturesResponse, error) {
	covenantPrivKey, err := kcs.getPrivKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get Covenant private key: %w", err)
	}

	slashSigs := make([][]byte, 0, len(req.FpEncKeys))
	slashUnbondingSigs := make([][]byte, 0, len(req.FpEncKeys))
	for _, fpEncKey := range req.FpEncKeys {
		slashSig, slashUnbondingSig, err := slashUnbondSig(covenantPrivKey, req, fpEncKey)
		if err != nil {
			return nil, err
		}

		slashSigs = append(slashSigs, slashSig.MustMarshal())
		slashUnbondingSigs = append(slashUnbondingSigs, slashUnbondingSig.MustMarshal())
	}

	unbondingSig, err := unbondSig(covenantPrivKey, req)
	if err != nil {
		return nil, err
	}

	return &covenant.SignaturesResponse{
		SlashSigs:          slashSigs,
		UnbondingSig:       unbondingSig,
		SlashUnbondingSigs: slashUnbondingSigs,
	}, nil
}

func slashUnbondSig(
	covenantPrivKey *secp.PrivateKey,
	signingTxReq covenant.SigningRequest,
	fpEncKey *asig.EncryptionKey,
) (slashSig, slashUnbondingSig *asig.AdaptorSignature, err error) {
	// creates slash sigs
	slashSig, err = btcstaking.EncSignTxWithOneScriptSpendInputStrict(
		signingTxReq.SlashingTx,
		signingTxReq.StakingTx,
		signingTxReq.StakingOutputIdx,
		signingTxReq.SlashingPkScriptPath,
		covenantPrivKey,
		fpEncKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign adaptor slash signature with finality provider public key %s: %w", fpEncKey.ToBytes(), err)
	}

	// creates slash unbonding sig
	slashUnbondingSig, err = btcstaking.EncSignTxWithOneScriptSpendInputStrict(
		signingTxReq.SlashUnbondingTx,
		signingTxReq.UnbondingTx,
		0, // 0th output is always the unbonding script output
		signingTxReq.UnbondingTxSlashingPkScriptPath,
		covenantPrivKey,
		fpEncKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign adaptor slash unbonding signature with finality provider public key %s: %w", fpEncKey.ToBytes(), err)
	}

	return slashSig, slashUnbondingSig, nil
}

func unbondSig(covenantPrivKey *secp.PrivateKey, signingTxReq covenant.SigningRequest) (*schnorr.Signature, error) {
	unbondingSig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
		signingTxReq.UnbondingTx,
		signingTxReq.StakingTx,
		signingTxReq.StakingOutputIdx,
		signingTxReq.StakingTxUnbondingPkScriptPath,
		covenantPrivKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sign unbonding tx: %w", err)
	}
	return unbondingSig, nil
}
