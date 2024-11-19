package signerapp_test

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/mocks"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/networks/parameters/parser"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

var (
	defaultParam = parser.VersionedGlobalParams{
		Version:          0,
		ActivationHeight: 100,
		StakingCap:       3000000,
		CapHeight:        0,
		Tag:              "01020304",
		CovenantPks: []string{
			"03ffeaec52a9b407b355ef6967a7ffc15fd6c3fe07de2844d61550475e7a5233e5",
			"03a5c60c2188e833d39d0fa798ab3f69aa12ed3dd2f3bad659effa252782de3c31",
			"0359d3532148a597a2d05c0395bf5f7176044b1cd312f37701a9b4d0aad70bc5a4",
			"0357349e985e742d5131e1e2b227b5170f6350ac2e2feb72254fcc25b3cee21a18",
			"03c8ccb03c379e452f10c81232b41a1ca8b63d0baf8387e57d302c987e5abb8527",
		},
		CovenantQuorum:    3,
		UnbondingTime:     1000,
		UnbondingFee:      1000,
		MaxStakingAmount:  300000,
		MinStakingAmount:  3000,
		MaxStakingTime:    10000,
		MinStakingTime:    100,
		ConfirmationDepth: 10,
	}

	globalParams = parser.GlobalParams{
		Versions: []*parser.VersionedGlobalParams{&defaultParam},
	}

	// always valid
	parsed, _ = parser.ParseGlobalParams(&globalParams)

	net = chaincfg.MainNetParams
)

type MockedDependencies struct {
	pr     *mocks.MockBabylonParamsRetriever
	bi     *mocks.MockBtcChainInfo
	s      *mocks.MockExternalBtcSigner
	params *signerapp.BabylonParams
}

func parserParamsToBabylonParams(
	versionedParams *parser.ParsedVersionedGlobalParams) *signerapp.BabylonParams {
	return &signerapp.BabylonParams{
		CovenantPublicKeys: versionedParams.CovenantPks,
		CovenantQuorum:     versionedParams.CovenantQuorum,
		MagicBytes:         versionedParams.Tag,
		UnbondingTime:      versionedParams.UnbondingTime,
		UnbondingFee:       versionedParams.UnbondingFee,
		MaxStakingAmount:   versionedParams.MaxStakingAmount,
		MinStakingAmount:   versionedParams.MinStakingAmount,
		MaxStakingTime:     versionedParams.MaxStakingTime,
		MinStakingTime:     versionedParams.MinStakingTime,
		ConfirmationDepth:  versionedParams.ConfirmationDepth,
	}
}

func NewMockedDependencies(t *testing.T) *MockedDependencies {
	ctrl := gomock.NewController(t)
	return &MockedDependencies{
		pr:     mocks.NewMockBabylonParamsRetriever(ctrl),
		bi:     mocks.NewMockBtcChainInfo(ctrl),
		s:      mocks.NewMockExternalBtcSigner(ctrl),
		params: parserParamsToBabylonParams(parsed.Versions[0]),
	}
}

type TestData struct {
	StakerPrivKey             *btcec.PrivateKey
	StakerPubKey              *btcec.PublicKey
	FinalityProviderPublicKey *btcec.PublicKey
	StakingInfo               *btcstaking.IdentifiableStakingInfo
	StakingTransaction        *wire.MsgTx
	UnbondingTx               *wire.MsgTx
	UnbondingTxStakerSig      *schnorr.Signature
}

func NewValidTestData(t *testing.T, params *signerapp.BabylonParams) *TestData {
	stakerKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	stakerPubKey := stakerKey.PubKey()
	fpKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	stakingInfo, stakingTx, err := btcstaking.BuildV0IdentifiableStakingOutputsAndTx(
		params.MagicBytes,
		stakerPubKey,
		fpKey.PubKey(),
		params.CovenantPublicKeys,
		params.CovenantQuorum,
		params.MinStakingTime+1,
		params.MaxStakingAmount,
		&net,
	)

	require.NoError(t, err)

	stakingUnbondingPathInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	fakeInputHashBytes := [32]byte{}
	fakeInputHash, err := chainhash.NewHash(fakeInputHashBytes[:])
	require.NoError(t, err)
	fakeInputIndex := uint32(0)
	stakingTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(fakeInputHash, fakeInputIndex), nil, nil))

	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		stakerPubKey,
		[]*btcec.PublicKey{fpKey.PubKey()},
		params.CovenantPublicKeys,
		params.CovenantQuorum,
		params.UnbondingTime,
		btcutil.Amount(stakingInfo.StakingOutput.Value-int64(params.UnbondingFee)),
		&net,
	)
	require.NoError(t, err)
	stakingTxHash := stakingTx.TxHash()
	unbondingTx := wire.NewMsgTx(wire.TxVersion)
	unbondingTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&stakingTxHash, 0), nil, nil))
	unbondingTx.AddTxOut(unbondingInfo.UnbondingOutput)

	validSig, err := btcstaking.SignTxWithOneScriptSpendInputFromTapLeaf(
		unbondingTx,
		stakingInfo.StakingOutput,
		stakerKey,
		stakingUnbondingPathInfo.RevealedLeaf,
	)

	require.NoError(t, err)

	return &TestData{
		StakerPrivKey:             stakerKey,
		StakerPubKey:              stakerPubKey,
		FinalityProviderPublicKey: fpKey.PubKey(),
		StakingInfo:               stakingInfo,
		StakingTransaction:        stakingTx,
		UnbondingTx:               unbondingTx,
		UnbondingTxStakerSig:      validSig,
	}
}

func TestValidSigningRequest(t *testing.T) {
	deps := NewMockedDependencies(t)
	signerApp := signerapp.NewSignerApp(deps.s, deps.bi, deps.pr, &net)
	validData := NewValidTestData(t, deps.params)

	deps.bi.EXPECT().TxByHash(
		gomock.Any(),
		&validData.UnbondingTx.TxIn[0].PreviousOutPoint.Hash,
		validData.StakingInfo.StakingOutput.PkScript).Return(
		&signerapp.TxInfo{
			Tx:                validData.StakingTransaction,
			TxInclusionHeight: 200,
		}, nil,
	)
	deps.bi.EXPECT().BestBlockHeight(gomock.Any()).Return(uint32(300), nil)
	deps.pr.EXPECT().ParamsByHeight(gomock.Any(), uint64(200)).Return(deps.params, nil)
	// return staker signature from mock, as it does not matter for test correctness
	deps.s.EXPECT().RawSignature(gomock.Any(), gomock.Any()).Return(&signerapp.SigningResult{
		Signature: validData.UnbondingTxStakerSig,
	}, nil)

	receivedSignature, err := signerApp.SignUnbondingTransaction(
		context.Background(),
		validData.StakingInfo.StakingOutput.PkScript,
		validData.UnbondingTx,
		validData.UnbondingTxStakerSig,
		deps.params.CovenantPublicKeys[0],
	)

	require.NoError(t, err)
	require.NotNil(t, receivedSignature)
	require.Equal(t, validData.UnbondingTxStakerSig, receivedSignature)
}

func TestErrRequestNotCovenantMember(t *testing.T) {
	deps := NewMockedDependencies(t)
	signerApp := signerapp.NewSignerApp(deps.s, deps.bi, deps.pr, &net)
	validData := NewValidTestData(t, deps.params)

	deps.bi.EXPECT().TxByHash(
		gomock.Any(),
		&validData.UnbondingTx.TxIn[0].PreviousOutPoint.Hash,
		validData.StakingInfo.StakingOutput.PkScript).Return(
		&signerapp.TxInfo{
			Tx:                validData.StakingTransaction,
			TxInclusionHeight: 200,
		}, nil,
	)
	deps.bi.EXPECT().BestBlockHeight(gomock.Any()).Return(uint32(300), nil)
	deps.pr.EXPECT().ParamsByHeight(gomock.Any(), uint64(200)).Return(deps.params, nil)

	unknownCovenantMember, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	receivedSignature, err := signerApp.SignUnbondingTransaction(
		context.Background(),
		validData.StakingInfo.StakingOutput.PkScript,
		validData.UnbondingTx,
		validData.UnbondingTxStakerSig,
		unknownCovenantMember.PubKey(),
	)

	require.Error(t, err)
	require.Nil(t, receivedSignature)
	require.True(t, errors.Is(err, signerapp.ErrInvalidSigningRequest))
}

func TestErrSignerNotReady(t *testing.T) {

	prvKey := "tprv8ZgxMBicQKsPdkArkw7uECTCfqthm5NAWhLpcHMyHYTAsKv2V3QsxvXhAyLqfjXSsdFAVAhwq54TsZe7rkYB3QCCNNVm4xHM7y8z8hoYzzk"
	hdKey, err := hdkeychain.NewKeyFromString(prvKey)
	require.NoError(t, err)
	fmt.Println(hdKey.String())
	ecPrivKey, err := hdKey.ECPrivKey()
	require.NoError(t, err)
	fmt.Println("Master key")
	fmt.Println(hex.EncodeToString(ecPrivKey.Serialize()))

	fmt.Println("Derive 0")
	key, err := DeriveDefaultWitnessKeyPath(hdKey, 0)
	require.NoError(t, err)
	fmt.Println(key.String())
	ecPrivKey1, err := key.ECPrivKey()
	require.NoError(t, err)
	fmt.Println("Private key")
	fmt.Println(hex.EncodeToString(ecPrivKey1.Serialize()))
	pubKey := ecPrivKey1.PubKey()
	fmt.Println("Public key")
	fmt.Println(hex.EncodeToString(pubKey.SerializeCompressed()))

}

// 84h/1h/0h/0
func DeriveDefaultWitnessKeyPath(masterPrivKey *hdkeychain.ExtendedKey, index uint32) (*hdkeychain.ExtendedKey, error) {

	// 84h
	first, err := masterPrivKey.Derive(hdkeychain.HardenedKeyStart + 84)
	if err != nil {
		return nil, err
	}
	// 84h/1h
	second, err := first.Derive(hdkeychain.HardenedKeyStart + 1)
	if err != nil {
		return nil, err
	}
	// 84h/1h/0h
	third, err := second.Derive(hdkeychain.HardenedKeyStart + 0)
	if err != nil {
		return nil, err
	}

	fourth, err := third.Derive(0)
	if err != nil {
		return nil, err
	}

	fifth, err := fourth.Derive(index)
	if err != nil {
		return nil, err
	}

	return fifth, nil
}
