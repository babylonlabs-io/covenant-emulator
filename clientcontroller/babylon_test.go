package clientcontroller_test

import (
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/babylonlabs-io/covenant-emulator/clientcontroller"
	"github.com/babylonlabs-io/covenant-emulator/testutil"
	"github.com/stretchr/testify/require"
)

func FuzzDelegationRespToDelegation(f *testing.F) {
	testutil.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))

		delPub, err := datagen.GenRandomBIP340PubKey(r)
		require.NoError(t, err)

		fpPub, err := datagen.GenRandomBIP340PubKey(r)
		require.NoError(t, err)

		randSat := datagen.RandomInRange(r, 10000, 10000000)
		randVersion := datagen.RandomInRange(r, 1, 10)
		stakingTx := datagen.GenRandomHexStr(r, 100)
		slashingTx := datagen.GenRandomHexStr(r, 100)
		stakingTime := datagen.RandomInRange(r, 100, 1000000)
		startHeight := datagen.RandomInRange(r, 100, 1000000)
		endHeight := datagen.RandomInRange(r, 100, 1000000)
		stakingOutputIdx := datagen.RandomInRange(r, 100, 1000000)

		response := types.BTCDelegationResponse{
			BtcPk:            delPub,
			FpBtcPkList:      []bbntypes.BIP340PubKey{*fpPub},
			TotalSat:         uint64(randSat),
			ParamsVersion:    uint32(randVersion),
			StakingTxHex:     stakingTx,
			SlashingTxHex:    slashingTx,
			StakingTime:      uint32(stakingTime),
			StartHeight:      uint32(startHeight),
			EndHeight:        uint32(endHeight),
			StakingOutputIdx: uint32(stakingOutputIdx),
		}

		del, err := clientcontroller.DelegationRespToDelegation(&response)
		require.NoError(t, err)
		require.NotNil(t, del)
		require.Equal(t, response.ParamsVersion, del.ParamsVersion)
		require.Equal(t, response.StakingTime, del.StakingTime)
		require.Equal(t, response.StartHeight, del.StartHeight)
		require.Equal(t, response.EndHeight, del.EndHeight)
		require.Equal(t, response.StakingOutputIdx, del.StakingOutputIdx)
		require.Equal(t, response.StakingTxHex, del.StakingTxHex)
		require.Equal(t, response.SlashingTxHex, del.SlashingTxHex)
		require.Equal(t, response.BtcPk, bbntypes.NewBIP340PubKeyFromBTCPK(del.BtcPk))
		require.Equal(t, response.FpBtcPkList, bbntypes.NewBIP340PKsFromBTCPKs(del.FpBtcPks))
	})
}
