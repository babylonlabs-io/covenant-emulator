package types

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

type ChainKeyInfo struct {
	keys.KeyOutput
	PublicKey  *btcec.PublicKey
	PrivateKey *btcec.PrivateKey
}

func NewChainKeyInfo(
	k *keyring.Record,
	pk *secp256k1.PublicKey,
	sk *secp256k1.PrivateKey,
) (*ChainKeyInfo, error) {
	keyOut, err := keys.MkAccKeyOutput(k)
	if err != nil {
		return nil, err
	}
	return &ChainKeyInfo{
		KeyOutput:  keyOut,
		PublicKey:  pk,
		PrivateKey: sk,
	}, nil
}
