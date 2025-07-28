package covenant_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/babylonlabs-io/babylon/v3/btcstaking"
	asig "github.com/babylonlabs-io/babylon/v3/crypto/schnorr-adaptor-signature"
	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	covcfg "github.com/babylonlabs-io/covenant-emulator/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant"
	signerCfg "github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/keystore/cosmos"
	signerMetrics "github.com/babylonlabs-io/covenant-emulator/covenant-signer/observability/metrics"
	signerApp "github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice"
	"github.com/babylonlabs-io/covenant-emulator/keyring"
	"github.com/babylonlabs-io/covenant-emulator/remotesigner"
	"github.com/babylonlabs-io/covenant-emulator/testutil"
	"github.com/babylonlabs-io/covenant-emulator/types"
)

const (
	passphrase = "testpass"
	hdPath     = ""
)

var net = &chaincfg.SimNetParams

func FuzzAddCovenantSig(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)

	// create a Covenant key pair in the keyring
	covenantConfig := covcfg.DefaultConfig()

	covenantConfig.BabylonConfig.KeyDirectory = f.TempDir()

	signerConfig := signerCfg.DefaultConfig()
	signerConfig.KeyStore.CosmosKeyStore.ChainID = covenantConfig.BabylonConfig.ChainID
	signerConfig.KeyStore.CosmosKeyStore.KeyName = covenantConfig.BabylonConfig.Key
	signerConfig.KeyStore.CosmosKeyStore.KeyringBackend = covenantConfig.BabylonConfig.KeyringBackend
	signerConfig.KeyStore.CosmosKeyStore.KeyDirectory = covenantConfig.BabylonConfig.KeyDirectory
	keyRetriever, err := cosmos.NewCosmosKeyringRetriever(signerConfig.KeyStore.CosmosKeyStore)
	require.NoError(f, err)

	covKeyPair, err := keyRetriever.Kr.CreateChainKey(
		passphrase,
		hdPath,
	)
	require.NoError(f, err)
	require.NotNil(f, covKeyPair)

	app := signerApp.NewSignerApp(
		keyRetriever,
	)

	met := signerMetrics.NewCovenantSignerMetrics()
	parsedConfig, err := signerConfig.Parse()
	require.NoError(f, err)

	server, err := signerservice.New(
		context.Background(),
		parsedConfig,
		app,
		met,
	)
	require.NoError(f, err)

	signer := remotesigner.NewRemoteSigner(covenantConfig.RemoteSigner)

	go func() {
		_ = server.Start()
	}()

	// Give some time to launch server
	time.Sleep(time.Second)

	// unlock the signer before usage
	err = signerservice.Unlock(
		context.Background(),
		covenantConfig.RemoteSigner.URL,
		covenantConfig.RemoteSigner.Timeout,
		passphrase,
		covenantConfig.RemoteSigner.HMACKey,
	)
	require.NoError(f, err)

	f.Cleanup(func() {
		_ = server.Stop(context.TODO())
	})

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Log("Seed", seed)
		r := rand.New(rand.NewSource(seed))

		params := testutil.GenRandomParams(r, t)
		mockClientController := testutil.PrepareMockedClientController(t, params)

		// create and start covenant emulator
		ce, err := covenant.NewCovenantEmulator(&covenantConfig, mockClientController, zap.NewNop(), signer)
		require.NoError(t, err)

		numDels := datagen.RandomInt(r, 3) + 1
		covSigsSet := make([]*types.CovenantSigs, 0, numDels)
		btcDels := make([]*types.Delegation, 0, numDels)
		for i := 0; uint64(i) < numDels; i++ {
			// generate BTC delegation
			delSK, delPK, err := datagen.GenRandomBTCKeyPair(r)
			require.NoError(t, err)
			stakingTimeBlocks := uint32(testutil.RandRange(r, int(params.MinStakingTime), int(params.MaxStakingTime)))
			stakingValue := int64(testutil.RandRange(r, int(params.MinStakingValue), int(params.MaxStakingValue)))
			unbondingTime := uint16(params.UnbondingTimeBlocks)
			fpNum := datagen.RandomInt(r, 5) + 1
			fpPks := testutil.GenBtcPublicKeys(r, t, int(fpNum))
			testInfo := datagen.GenBTCStakingSlashingInfo(
				r,
				t,
				net,
				delSK,
				fpPks,
				params.CovenantPks,
				params.CovenantQuorum,
				uint16(stakingTimeBlocks),
				stakingValue,
				params.SlashingPkScript,
				params.SlashingRate,
				unbondingTime,
			)
			stakingTxBytes, err := bbntypes.SerializeBTCTx(testInfo.StakingTx)
			require.NoError(t, err)
			startHeight := uint32(datagen.RandomInt(r, 1000) + 100)
			stakingOutputIdx, err := bbntypes.GetOutputIdxInBTCTx(testInfo.StakingTx, testInfo.StakingInfo.StakingOutput)
			require.NoError(t, err)
			randParamsVersion := datagen.RandomInRange(r, 1, 10)
			btcDel := &types.Delegation{
				BtcPk:            delPK,
				FpBtcPks:         fpPks,
				StakingTime:      stakingTimeBlocks,
				StartHeight:      startHeight,
				EndHeight:        startHeight + stakingTimeBlocks,
				TotalSat:         btcutil.Amount(stakingValue),
				UnbondingTime:    unbondingTime,
				StakingTxHex:     hex.EncodeToString(stakingTxBytes),
				StakingOutputIdx: stakingOutputIdx,
				SlashingTxHex:    testInfo.SlashingTx.ToHexStr(),
				ParamsVersion:    uint32(randParamsVersion),
			}
			btcDels = append(btcDels, btcDel)
			// generate covenant staking sigs
			slashingSpendInfo, err := testInfo.StakingInfo.SlashingPathSpendInfo()
			require.NoError(t, err)
			covSigs := make([][]byte, 0, len(fpPks))
			for _, fpPk := range fpPks {
				encKey, err := asig.NewEncryptionKeyFromBTCPK(fpPk)
				require.NoError(t, err)
				covenantSig, err := testInfo.SlashingTx.EncSign(
					testInfo.StakingTx,
					0,
					slashingSpendInfo.GetPkScriptPath(),
					covKeyPair.PrivateKey, encKey,
				)
				require.NoError(t, err)
				covSigs = append(covSigs, covenantSig.MustMarshal())
			}

			// generate undelegation
			unbondingValue := int64(btcDel.TotalSat) - int64(params.UnbondingFee)

			stakingTxHash := testInfo.StakingTx.TxHash()
			testUnbondingInfo := datagen.GenBTCUnbondingSlashingInfo(
				r,
				t,
				net,
				delSK,
				btcDel.FpBtcPks,
				params.CovenantPks,
				params.CovenantQuorum,
				wire.NewOutPoint(&stakingTxHash, 0),
				unbondingTime,
				unbondingValue,
				params.SlashingPkScript,
				params.SlashingRate,
				unbondingTime,
			)
			require.NoError(t, err)
			// random signer
			unbondingTxMsg := testUnbondingInfo.UnbondingTx

			unbondingSlashingPathInfo, err := testUnbondingInfo.UnbondingInfo.SlashingPathSpendInfo()
			require.NoError(t, err)

			serializedUnbondingTx, err := bbntypes.SerializeBTCTx(testUnbondingInfo.UnbondingTx)
			require.NoError(t, err)
			undel := &types.Undelegation{
				UnbondingTxHex: hex.EncodeToString(serializedUnbondingTx),
				SlashingTxHex:  testUnbondingInfo.SlashingTx.ToHexStr(),
			}
			btcDel.BtcUndelegation = undel
			stakingTxUnbondingPathInfo, err := testInfo.StakingInfo.UnbondingPathSpendInfo()
			require.NoError(t, err)
			// generate covenant unbonding sigs
			unbondingCovSig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
				unbondingTxMsg,
				testInfo.StakingTx,
				btcDel.StakingOutputIdx,
				stakingTxUnbondingPathInfo.GetPkScriptPath(),
				covKeyPair.PrivateKey,
			)
			require.NoError(t, err)
			// generate covenant unbonding slashing sigs
			unbondingCovSlashingSigs := make([][]byte, 0, len(fpPks))
			for _, fpPk := range fpPks {
				encKey, err := asig.NewEncryptionKeyFromBTCPK(fpPk)
				require.NoError(t, err)
				covenantSig, err := testUnbondingInfo.SlashingTx.EncSign(
					testUnbondingInfo.UnbondingTx,
					0,
					unbondingSlashingPathInfo.GetPkScriptPath(),
					covKeyPair.PrivateKey,
					encKey,
				)
				require.NoError(t, err)
				unbondingCovSlashingSigs = append(unbondingCovSlashingSigs, covenantSig.MustMarshal())
			}
			covSigsSet = append(covSigsSet, &types.CovenantSigs{
				PublicKey:             covKeyPair.PublicKey,
				StakingTxHash:         testInfo.StakingTx.TxHash(),
				SlashingSigs:          covSigs,
				UnbondingSig:          unbondingCovSig,
				SlashingUnbondingSigs: unbondingCovSlashingSigs,
			})
		}

		// add one invalid delegation to expect it does not affect others
		invalidDelegation := &types.Delegation{
			StakingTxHex: "xxxx",
		}
		btcDels = append(btcDels, invalidDelegation)

		// check the sigs are expected
		expectedTxHash := testutil.GenRandomHexStr(r, 32)
		mockClientController.EXPECT().SubmitCovenantSigs(gomock.Any()).
			Return(&types.TxResponse{TxHash: expectedTxHash}, nil).AnyTimes()
		res, err := ce.AddCovenantSignatures(btcDels)
		require.NoError(t, err)
		require.Equal(t, expectedTxHash, res.TxHash, "covSigsSet %+v didn't result in expected tx hash", covSigsSet)
	})
}

func TestDeduplicationWithOddKey(t *testing.T) {
	// 1. Public key with odd y coordinate
	oddKey := "0379a71ffd71c503ef2e2f91bccfc8fcda7946f4653cef0d9f3dde20795ef3b9f0"
	oddKeyBytes, err := hex.DecodeString(oddKey)
	require.NoError(t, err)
	oddKeyPub, err := btcec.ParsePubKey(oddKeyBytes)
	require.NoError(t, err)

	// 2. Serialize in BTC schnorr format
	serializedOddKey := schnorr.SerializePubKey(oddKeyPub)
	pubKeyFromSchnorr, err := schnorr.ParsePubKey(serializedOddKey)
	require.NoError(t, err)

	randomKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	randPubKey := randomKey.PubKey()

	paramVersion := uint32(2)
	delAlreadySigned := &types.Delegation{
		CovenantSigs: []*types.CovenantAdaptorSigInfo{
			&types.CovenantAdaptorSigInfo{
				// 3. Delegation is already signed by the public key with odd y coordinate
				Pk: pubKeyFromSchnorr,
			},
		},
		ParamsVersion: paramVersion,
	}

	delNotSigned := &types.Delegation{
		CovenantSigs: []*types.CovenantAdaptorSigInfo{
			&types.CovenantAdaptorSigInfo{
				Pk: randPubKey,
			},
		},
		ParamsVersion: paramVersion,
	}

	paramsGet := NewMockParam(map[uint32]*types.StakingParams{
		paramVersion: &types.StakingParams{
			CovenantPks: []*secp256k1.PublicKey{oddKeyPub, pubKeyFromSchnorr},
		},
	})

	// 4. After removing the already signed delegation, the list should have only one element
	accept, err := covenant.AcceptDelegationToSign(oddKeyPub, paramsGet, delAlreadySigned, nil)
	require.NoError(t, err)
	require.False(t, accept)

	accept, err = covenant.AcceptDelegationToSign(oddKeyPub, paramsGet, delNotSigned, nil)
	require.NoError(t, err)
	require.True(t, accept)
}

func TestIsKeyInCommittee(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().Unix()))

	// create a Covenant key pair in the keyring
	covenantConfig := covcfg.DefaultConfig()
	covenantConfig.BabylonConfig.KeyDirectory = t.TempDir()

	covKeyPair, err := keyring.CreateCovenantKey(
		covenantConfig.BabylonConfig.KeyDirectory,
		covenantConfig.BabylonConfig.ChainID,
		covenantConfig.BabylonConfig.Key,
		covenantConfig.BabylonConfig.KeyringBackend,
		passphrase,
		hdPath,
	)
	require.NoError(t, err)
	covenantSerializedPk := schnorr.SerializePubKey(covKeyPair.PublicKey)

	// create params and version
	pVersionWithoutCovenant := uint32(datagen.RandomInRange(r, 1, 10))
	pVersionWithCovenant := pVersionWithoutCovenant + 1

	paramsWithoutCovenant := testutil.GenRandomParams(r, t)
	paramsWithCovenant := testutil.GenRandomParams(r, t)
	paramsWithCovenant.CovenantPks = append(paramsWithCovenant.CovenantPks, covKeyPair.PublicKey)

	// creates delegations to check
	delNoCovenant := &types.Delegation{
		ParamsVersion: pVersionWithoutCovenant,
	}
	delWithCovenant := &types.Delegation{
		ParamsVersion: pVersionWithCovenant,
	}

	paramsGet := NewMockParam(map[uint32]*types.StakingParams{
		pVersionWithoutCovenant: paramsWithoutCovenant,
		pVersionWithCovenant:    paramsWithCovenant,
	})

	// checks the case where the covenant is NOT in the committee
	actual, err := covenant.IsKeyInCommittee(paramsGet, covenantSerializedPk, delNoCovenant)
	require.False(t, actual)
	require.NoError(t, err)

	accept, err := covenant.AcceptDelegationToSign(covKeyPair.PublicKey, paramsGet, delNoCovenant, nil)
	require.NoError(t, err)
	require.False(t, accept)

	// checks the case where the covenant is in the committee
	actual, err = covenant.IsKeyInCommittee(paramsGet, covenantSerializedPk, delWithCovenant)
	require.True(t, actual)
	require.NoError(t, err)

	accept, err = covenant.AcceptDelegationToSign(covKeyPair.PublicKey, paramsGet, delWithCovenant, nil)
	require.NoError(t, err)
	require.True(t, accept)

	errParamGet := fmt.Errorf("dumbErr")
	accept, err = covenant.AcceptDelegationToSign(covKeyPair.PublicKey, NewMockParamError(errParamGet), delWithCovenant, nil)
	require.False(t, accept)

	errKeyIsInCommittee := fmt.Errorf("unable to get the param version: %d, reason: %s", pVersionWithCovenant, errParamGet.Error())
	expErr := fmt.Errorf("unable to verify if covenant key is in committee: %s", errKeyIsInCommittee.Error())
	require.EqualError(t, err, expErr.Error())
}

type MockParamGetter struct {
	paramsByVersion map[uint32]*types.StakingParams
}

func NewMockParam(p map[uint32]*types.StakingParams) *MockParamGetter {
	return &MockParamGetter{
		paramsByVersion: p,
	}
}

func (m *MockParamGetter) Get(version uint32) (*types.StakingParams, error) {
	p := m.paramsByVersion[version]
	return p, nil
}

type MockParamError struct {
	err error
}

func NewMockParamError(err error) *MockParamError {
	return &MockParamError{
		err: err,
	}
}

func (m *MockParamError) Get(version uint32) (*types.StakingParams, error) {
	return nil, m.err
}

func TestValidateStakeExpansion(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().Unix()))

	covenantConfig := covcfg.DefaultConfig()
	covenantConfig.BabylonConfig.KeyDirectory = t.TempDir()

	covKeyPairInCommittee, err := keyring.CreateCovenantKey(
		covenantConfig.BabylonConfig.KeyDirectory,
		covenantConfig.BabylonConfig.ChainID,
		covenantConfig.BabylonConfig.Key,
		covenantConfig.BabylonConfig.KeyringBackend,
		passphrase,
		hdPath,
	)
	require.NoError(t, err)

	covKeyInCommittee := schnorr.SerializePubKey(covKeyPairInCommittee.PublicKey)
	pubKeyFromSchnorr, err := schnorr.ParsePubKey(covKeyInCommittee)
	require.NoError(t, err)

	badCovKey, err := keyring.CreateCovenantKey(
		covenantConfig.BabylonConfig.KeyDirectory,
		covenantConfig.BabylonConfig.ChainID,
		"other-covenant-key",
		covenantConfig.BabylonConfig.KeyringBackend,
		passphrase,
		hdPath,
	)
	require.NoError(t, err)
	covKeyNotInCommittee := schnorr.SerializePubKey(badCovKey.PublicKey)

	pVersionWithoutCovenant := uint32(datagen.RandomInRange(r, 1, 10))
	pVersionWithCovenant := pVersionWithoutCovenant + 1

	paramsWithoutCovenant := testutil.GenRandomParams(r, t)
	paramsWithCovenant := testutil.GenRandomParams(r, t)
	paramsWithCovenant.CovenantPks = append(paramsWithCovenant.CovenantPks, covKeyPairInCommittee.PublicKey)

	paramsGet := NewMockParam(map[uint32]*types.StakingParams{
		pVersionWithoutCovenant: paramsWithoutCovenant,
		pVersionWithCovenant:    paramsWithCovenant,
	})

	stkTxHex := "02000000012c1ca601b81bf5bdd97081d1bf17241d4d688f51ccbe8be3d3f3174d0e4e4aa40100000000ffffffff0250c3000000000000225120d0d55103aa70a12162f733805c3a2f5ff8e857d5fc92381c3d6f22a791165ac115400f00000000002251206f5ec73002ee8b5b2bb942f26e169354821e6ec06f9b3a1d3cf355d6f276c5d800000000"
	stakingMsgTx, _, err := bbntypes.NewBTCTxFromHex(stkTxHex)
	require.NoError(t, err)

	prevStakingTxHex := stakingMsgTx.TxHash().String()
	prevDel := &types.Delegation{
		StakingTxHex:  stkTxHex,
		ParamsVersion: pVersionWithCovenant,
		CovenantSigs:  []*types.CovenantAdaptorSigInfo{},
	}
	stkExp := &types.Delegation{
		StakingTxHex:  testutil.GenRandomHexStr(r, 100),
		ParamsVersion: pVersionWithCovenant,
		CovenantSigs:  []*types.CovenantAdaptorSigInfo{},
		StakeExpansion: &types.DelegationStakeExpansion{
			PreviousStakingTxHashHex: prevStakingTxHex,
		},
	}

	t.Run("successful validation - covenant in previous committee and didn't sign", func(t *testing.T) {
		valid, err := covenant.ValidateStakeExpansion(paramsGet, covKeyInCommittee, stkExp, prevDel)
		require.NoError(t, err)
		require.True(t, valid, "should be valid when covenant was in previous committee, even if it didn't sign")
	})

	prevDel.CovenantSigs = append(prevDel.CovenantSigs, &types.CovenantAdaptorSigInfo{
		Pk: pubKeyFromSchnorr,
	})
	t.Run("successful validation - covenant in previous committee and signed old del", func(t *testing.T) {
		valid, err := covenant.ValidateStakeExpansion(paramsGet, covKeyInCommittee, stkExp, prevDel)
		require.NoError(t, err)
		require.True(t, valid, "should be valid when covenant was in previous committee even if it did sign the previous")
	})

	t.Run("fails when covenant never in previous committee", func(t *testing.T) {
		valid, err := covenant.ValidateStakeExpansion(paramsGet, covKeyNotInCommittee, stkExp, prevDel)
		require.NoError(t, err)
		require.False(t, valid, "should fail when covenant was never in previous committee")
	})

	prevDel.ParamsVersion = pVersionWithoutCovenant
	t.Run("fails when covenant not in previous committee", func(t *testing.T) {
		valid, err := covenant.ValidateStakeExpansion(paramsGet, covKeyInCommittee, stkExp, prevDel)
		require.NoError(t, err)
		require.False(t, valid, "should fail when covenant was not in previous committee")
	})

	t.Run("fails when previous delegation is nil", func(t *testing.T) {
		valid, err := covenant.ValidateStakeExpansion(paramsGet, covKeyInCommittee, stkExp, nil)
		require.False(t, valid)
		require.EqualError(t, err, fmt.Sprintf("previous delegation is nil for stake expansion delegation: %s", prevStakingTxHex))
	})

	t.Run("fails when transaction hash mismatch", func(t *testing.T) {
		wrongHash := datagen.GenRandomHexStr(r, 10)
		mismatchedStkExp := *stkExp
		mismatchedStkExp.StakeExpansion.PreviousStakingTxHashHex = wrongHash

		valid, err := covenant.ValidateStakeExpansion(paramsGet, covKeyInCommittee, &mismatchedStkExp, prevDel)
		require.False(t, valid)
		require.EqualError(t, err, fmt.Sprintf("previous delegation staking tx hash mismatch: expected %s, got %s", wrongHash, prevStakingTxHex))
	})

	t.Run("fails when previous delegation has invalid staking tx", func(t *testing.T) {
		invalidPrevDel := &types.Delegation{
			StakingTxHex:  "invalid-hex-string",
			ParamsVersion: pVersionWithCovenant,
		}

		valid, err := covenant.ValidateStakeExpansion(paramsGet, covKeyInCommittee, stkExp, invalidPrevDel)
		require.False(t, valid)
		require.EqualError(t, err, "failed to decode previous delegation staking tx: encoding/hex: invalid byte: U+0069 'i'")
	})

	t.Run("fails when param cache returns error", func(t *testing.T) {
		cacheErr := fmt.Errorf("param cache connection error")
		errorParamCache := NewMockParamError(cacheErr)

		valid, err := covenant.ValidateStakeExpansion(errorParamCache, covKeyInCommittee, stkExp, prevDel)
		require.False(t, valid)
		require.EqualError(t, err, fmt.Sprintf("unable to verify if covenant key is in committee: unable to get the param version: %d, reason: %s", pVersionWithoutCovenant, cacheErr.Error()))
	})
}
