package signerapp

import (
	"context"
	"fmt"

	"github.com/babylonlabs-io/babylon/btcstaking"
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/wire"
)

type ParsedSigningRequest struct {
	StakingTx               *wire.MsgTx
	SlashingTx              *wire.MsgTx
	UnbondingTx             *wire.MsgTx
	SlashUnbondingTx        *wire.MsgTx
	StakingOutputIdx        uint32
	SlashingScript          []byte
	UnbondingScript         []byte
	UnbondingSlashingScript []byte
	FpEncKeys               []*asig.EncryptionKey
}

type ParsedSigningResponse struct {
	SlashAdaptorSigs          [][]byte
	UnbondingSig              *schnorr.Signature
	SlashUnbondingAdaptorSigs [][]byte
}

type SignerApp struct {
	pkr PrivKeyRetriever
}

func NewSignerApp(
	pkr PrivKeyRetriever,
) *SignerApp {
	return &SignerApp{
		pkr: pkr,
	}
}
func (s *SignerApp) SignTransactions(
	ctx context.Context,
	req *ParsedSigningRequest,
) (*ParsedSigningResponse, error) {
	privKey, err := s.pkr.PrivKey(ctx)

	if err != nil {
		return nil, err
	}

	slashSigs := make([][]byte, 0, len(req.FpEncKeys))
	slashUnbondingSigs := make([][]byte, 0, len(req.FpEncKeys))
	for _, fpEncKey := range req.FpEncKeys {
		slashSig, slashUnbondingSig, err := slashUnbondSig(privKey, req, fpEncKey)
		if err != nil {
			return nil, err
		}

		slashSigs = append(slashSigs, slashSig.MustMarshal())
		slashUnbondingSigs = append(slashUnbondingSigs, slashUnbondingSig.MustMarshal())
	}

	unbondingSig, err := unbondSig(privKey, req)
	if err != nil {
		return nil, err
	}

	return &ParsedSigningResponse{
		SlashAdaptorSigs:          slashSigs,
		UnbondingSig:              unbondingSig,
		SlashUnbondingAdaptorSigs: slashUnbondingSigs,
	}, nil
}

func (s *SignerApp) Unlock(ctx context.Context, passphrase string) error {
	return s.pkr.Unlock(ctx, passphrase)
}

func (s *SignerApp) Lock(ctx context.Context) error {
	return s.pkr.Lock(ctx)
}

func (s *SignerApp) PubKey(ctx context.Context) (*btcec.PublicKey, error) {
	privKey, err := s.pkr.PrivKey(ctx)
	if err != nil {
		return nil, err
	}

	return privKey.PubKey(), nil
}

func slashUnbondSig(
	covenantPrivKey *btcec.PrivateKey,
	signingTxReq *ParsedSigningRequest,
	fpEncKey *asig.EncryptionKey,
) (slashSig, slashUnbondingSig *asig.AdaptorSignature, err error) {
	// creates slash sigs
	slashSig, err = btcstaking.EncSignTxWithOneScriptSpendInputStrict(
		signingTxReq.SlashingTx,
		signingTxReq.StakingTx,
		signingTxReq.StakingOutputIdx,
		signingTxReq.SlashingScript,
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
		signingTxReq.UnbondingSlashingScript,
		covenantPrivKey,
		fpEncKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign adaptor slash unbonding signature with finality provider public key %s: %w", fpEncKey.ToBytes(), err)
	}

	return slashSig, slashUnbondingSig, nil
}

func unbondSig(covenantPrivKey *btcec.PrivateKey, signingTxReq *ParsedSigningRequest) (*schnorr.Signature, error) {
	unbondingSig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
		signingTxReq.UnbondingTx,
		signingTxReq.StakingTx,
		signingTxReq.StakingOutputIdx,
		signingTxReq.UnbondingScript,
		covenantPrivKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sign unbonding tx: %w", err)
	}
	return unbondingSig, nil
}
