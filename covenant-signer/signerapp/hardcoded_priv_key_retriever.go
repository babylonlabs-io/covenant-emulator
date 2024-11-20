package signerapp

import (
	"context"

	"github.com/btcsuite/btcd/btcec/v2"
)

var _ PrivKeyRetriever = &HardcodedPrivKeyRetriever{}

// HardcodedPrivKeyRetriever should only be used for test purposes
type HardcodedPrivKeyRetriever struct {
	privKey *btcec.PrivateKey
}

func NewHardcodedPrivKeyRetriever(privKey *btcec.PrivateKey) *HardcodedPrivKeyRetriever {
	return &HardcodedPrivKeyRetriever{
		privKey: privKey,
	}
}

func (r *HardcodedPrivKeyRetriever) PrivKey(ctx context.Context) (*btcec.PrivateKey, error) {
	// return copy of the private key
	bytes := r.privKey.Serialize()

	newPrivKey, _ := btcec.PrivKeyFromBytes(bytes)

	return newPrivKey, nil
}
