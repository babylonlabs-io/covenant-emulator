package covenant_test

import (
	"encoding/hex"
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/btcutil"

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
	"github.com/babylonlabs-io/covenant-emulator/keyring"
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
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Log("Seed", seed)
		r := rand.New(rand.NewSource(seed))

		params := testutil.GenRandomParams(r, t)
		mockClientController := testutil.PrepareMockedClientController(t, params)

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

		signer, err := keyring.NewKeyringSigner(covenantConfig.BabylonConfig.ChainID, covenantConfig.BabylonConfig.Key, covenantConfig.BabylonConfig.KeyDirectory, covenantConfig.BabylonConfig.KeyringBackend, passphrase)
		require.NoError(t, err)

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
			btcDel := &types.Delegation{
				BtcPk:            delPK,
				FpBtcPks:         fpPks,
				StakingTime:      stakingTimeBlocks,
				StartHeight:      startHeight, // not relevant here
				EndHeight:        startHeight + stakingTimeBlocks,
				TotalSat:         btcutil.Amount(stakingValue),
				UnbondingTime:    unbondingTime,
				StakingTxHex:     hex.EncodeToString(stakingTxBytes),
				StakingOutputIdx: stakingOutputIdx,
				SlashingTxHex:    testInfo.SlashingTx.ToHexStr(),
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
