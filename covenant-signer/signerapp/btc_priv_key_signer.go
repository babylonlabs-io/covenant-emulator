package signerapp

import (
	"context"
	"fmt"

	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/btcclient"
)

// PrivKeySigner is a signer that uses a private key from connected bitcoind node
// Due to transfer of key through channer, it require encrypted connection
// to bitcoind node like ssh or tls.
// Key is zeroed after signing, to not sit in memory longer than needed.
type PrivKeySigner struct {
	client *btcclient.BtcClient
}

func NewPrivKeySigner(client *btcclient.BtcClient) *PrivKeySigner {
	return &PrivKeySigner{
		client: client,
	}
}

var _ ExternalBtcSigner = (*PrivKeySigner)(nil)

func (s *PrivKeySigner) RawSignature(ctx context.Context, request *SigningRequest) (*SigningResult, error) {
	if err := btcstaking.IsSimpleTransfer(request.UnbondingTransaction); err != nil {
		return nil, fmt.Errorf("invalid unbonding transaction received for signing: %w", err)
	}

	key, err := s.client.DumpPrivateKey(request.CovenantAddress)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve covenant key for signing: %w", err)
	}
	// Zero key after signing
	defer key.Zero()

	sig, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		request.UnbondingTransaction,
		request.StakingOutput,
		key,
		*request.SpendDescription.ScriptLeaf,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return &SigningResult{
		Signature: sig,
	}, nil
}
