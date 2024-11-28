package signerapp

import (
	"context"

	"github.com/btcsuite/btcd/btcec/v2"
)

// PrivKeyRetriever is an interface that retrieves a private key, that must do
// the signing
type PrivKeyRetriever interface {
	PrivKey(ctx context.Context) (*btcec.PrivateKey, error)
	Unlock(ctx context.Context, passphrase string) error
	Lock(ctx context.Context) error
}
