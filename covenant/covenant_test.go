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

	"github.com/babylonlabs-io/babylon/btcstaking"
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
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
		mockClientController.EXPECT().SubmitCovenantSigs(covSigsSet).
			Return(&types.TxResponse{TxHash: expectedTxHash}, nil).AnyTimes()
		res, err := ce.AddCovenantSignatures(btcDels)
		require.NoError(t, err)
		require.Equal(t, expectedTxHash, res.TxHash)
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
	accept, err := covenant.AcceptDelegationToSign(oddKeyPub, paramsGet, delAlreadySigned)
	require.NoError(t, err)
	require.False(t, accept)

	accept, err = covenant.AcceptDelegationToSign(oddKeyPub, paramsGet, delNotSigned)
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

	// simple mock with the parameter versions
	paramsGet := NewMockParam(map[uint32]*types.StakingParams{
		pVersionWithoutCovenant: paramsWithoutCovenant,
		pVersionWithCovenant:    paramsWithCovenant,
	})

	// checks the case where the covenant is NOT in the committee
	actual, err := covenant.IsKeyInCommittee(paramsGet, covenantSerializedPk, delNoCovenant)
	require.False(t, actual)
	require.NoError(t, err)

	accept, err := covenant.AcceptDelegationToSign(covKeyPair.PublicKey, paramsGet, delNoCovenant)
	require.NoError(t, err)
	require.False(t, accept)

	// checks the case where the covenant is in the committee
	actual, err = covenant.IsKeyInCommittee(paramsGet, covenantSerializedPk, delWithCovenant)
	require.True(t, actual)
	require.NoError(t, err)

	accept, err = covenant.AcceptDelegationToSign(covKeyPair.PublicKey, paramsGet, delWithCovenant)
	require.NoError(t, err)
	require.True(t, accept)

	errParamGet := fmt.Errorf("dumbErr")
	accept, err = covenant.AcceptDelegationToSign(covKeyPair.PublicKey, NewMockParamError(errParamGet), delWithCovenant)
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
