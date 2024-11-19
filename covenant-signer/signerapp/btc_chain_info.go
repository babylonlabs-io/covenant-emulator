package signerapp

import (
	"context"
	"fmt"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/btcclient"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

var _ BtcChainInfo = (*BitcoindChainInfo)(nil)

type BitcoindChainInfo struct {
	c *btcclient.BtcClient
}

func NewBitcoindChainInfo(c *btcclient.BtcClient) *BitcoindChainInfo {
	return &BitcoindChainInfo{c: c}
}

func (b *BitcoindChainInfo) TxByHash(_ context.Context, txHash *chainhash.Hash, pkScript []byte) (*TxInfo, error) {
	conf, status, err := b.c.TxDetails(txHash, pkScript)

	if err != nil {
		return nil, fmt.Errorf("failed to get tx by hash: %w", err)
	}

	if status != btcclient.TxInChain {
		return nil, fmt.Errorf("tx with hash %s is not in chain", txHash.String())
	}

	return &TxInfo{
		Tx:                conf.Tx,
		TxInclusionHeight: conf.BlockHeight,
	}, nil
}

func (b *BitcoindChainInfo) BestBlockHeight(_ context.Context) (uint32, error) {
	return b.c.BestBlockHeight()
}
