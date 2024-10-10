package keyring

import (
	"fmt"
	"strings"

	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/covenant-emulator/covenant"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
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

// SignTransactions receives a batch of transactions to sign and returns all the signatures if nothing fails.
func (kcs KeyringSigner) SignTransactions(req covenant.SigningRequest) (*covenant.SigningResponse, error) {
	resp := make(map[chainhash.Hash]covenant.SignaturesResponse, len(req.SigningTxsReqByStkTxHash))

	covenantPrivKey, err := kcs.getPrivKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get Covenant private key: %w", err)
	}

	for stakingTxHash, signingTxReq := range req.SigningTxsReqByStkTxHash {
		// for each signing tx request

		slashSigs := make([][]byte, 0, len(signingTxReq.FpEncKeys))
		slashUnbondingSigs := make([][]byte, 0, len(signingTxReq.FpEncKeys))
		for _, fpEncKey := range signingTxReq.FpEncKeys {
			// creates slash sigs
			// TODO: split to diff func
			slashSig, err := btcstaking.EncSignTxWithOneScriptSpendInputStrict(
				signingTxReq.SlashingTx,
				signingTxReq.StakingTx,
				signingTxReq.StakingOutputIdx,
				signingTxReq.SlashingPkScriptPath,
				covenantPrivKey,
				fpEncKey,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to sign adaptor slash signature with finality provider public key %s: %w", fpEncKey.ToBytes(), err)
			}
			slashSigs = append(slashSigs, slashSig.MustMarshal())

			// TODO: split to diff func
			// creates slash unbonding sig
			slashUnbondingSig, err := btcstaking.EncSignTxWithOneScriptSpendInputStrict(
				signingTxReq.SlashUnbondingTx,
				signingTxReq.UnbondingTx,
				0, // 0th output is always the unbonding script output
				signingTxReq.UnbondingTxSlashingPkScriptPath,
				covenantPrivKey,
				fpEncKey,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to sign adaptor slash unbonding signature with finality provider public key %s: %w", fpEncKey.ToBytes(), err)
			}
			slashUnbondingSigs = append(slashUnbondingSigs, slashUnbondingSig.MustMarshal())
		}

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

		resp[stakingTxHash] = covenant.SignaturesResponse{
			SlashSigs:          slashSigs,
			UnbondingSig:       unbondingSig,
			SlashUnbondingSigs: slashUnbondingSigs,
		}
	}

	return &covenant.SigningResponse{
		SignaturesByStkTxHash: resp,
	}, nil
}
