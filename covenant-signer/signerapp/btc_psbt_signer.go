package signerapp

import (
	"context"
	"fmt"

	staking "github.com/babylonlabs-io/babylon/btcstaking"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/btcclient"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

var _ ExternalBtcSigner = (*PsbtSigner)(nil)

type PsbtSigner struct {
	client *btcclient.BtcClient
}

func NewPsbtSigner(client *btcclient.BtcClient) *PsbtSigner {
	return &PsbtSigner{
		client: client,
	}
}

// TODO: Figure out how to sign complex taproot scripts using psbt packets sent
// to bitcoind. It may require using descriptors wallets.
func (s *PsbtSigner) RawSignature(ctx context.Context, request *SigningRequest) (*SigningResult, error) {
	if err := staking.IsSimpleTransfer(request.UnbondingTransaction); err != nil {
		return nil, fmt.Errorf("invalid unbonding transaction: %w", err)
	}

	psbtPacket, err := psbt.New(
		[]*wire.OutPoint{&request.UnbondingTransaction.TxIn[0].PreviousOutPoint},
		request.UnbondingTransaction.TxOut,
		request.UnbondingTransaction.Version,
		request.UnbondingTransaction.LockTime,
		[]uint32{wire.MaxTxInSequenceNum},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create PSBT packet with unbonding transaction: %w", err)
	}

	psbtPacket.Inputs[0].SighashType = txscript.SigHashDefault
	psbtPacket.Inputs[0].WitnessUtxo = request.StakingOutput
	psbtPacket.Inputs[0].Bip32Derivation = []*psbt.Bip32Derivation{
		{
			PubKey: request.CovenantPublicKey.SerializeCompressed(),
		},
	}

	ctrlBlockBytes, err := request.SpendDescription.ControlBlock.ToBytes()

	if err != nil {
		return nil, fmt.Errorf("failed to serialize control block: %w", err)
	}

	psbtPacket.Inputs[0].TaprootLeafScript = []*psbt.TaprootTapLeafScript{
		{
			ControlBlock: ctrlBlockBytes,
			Script:       request.SpendDescription.ScriptLeaf.Script,
			LeafVersion:  request.SpendDescription.ScriptLeaf.LeafVersion,
		},
	}

	signedPacket, err := s.client.SignPsbt(psbtPacket)

	if err != nil {
		return nil, fmt.Errorf("failed to sign PSBT packet: %w", err)
	}

	if len(signedPacket.Inputs[0].TaprootScriptSpendSig) == 0 {
		// this can happen if btcwallet does not maintain the private key for the
		// for the public in signing request
		return nil, fmt.Errorf("no signature found in PSBT packet. Wallet does not maintain covenant public key")
	}

	schnorSignature := signedPacket.Inputs[0].TaprootScriptSpendSig[0].Signature

	parsedSignature, err := schnorr.ParseSignature(schnorSignature)

	if err != nil {
		return nil, fmt.Errorf("failed to parse schnorr signature in psbt packet: %w", err)

	}

	result := &SigningResult{
		Signature: parsedSignature,
	}

	return result, nil
}
