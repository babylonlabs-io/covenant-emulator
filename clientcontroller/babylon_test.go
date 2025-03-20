package clientcontroller_test

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/babylonlabs-io/covenant-emulator/clientcontroller"
	"github.com/babylonlabs-io/covenant-emulator/testutil"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
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

func TestFailedMessageIndex(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		title       string
		err         error
		expMsgIndex int
		expFound    bool
	}{
		{
			title:       "Valid message index in error",
			err:         errors.New("transaction failed: message index: 2"),
			expMsgIndex: 2,
			expFound:    true,
		},
		{
			title:       "Valid message index with extra spaces",
			err:         errors.New("message index:    15 "),
			expMsgIndex: 15,
			expFound:    true,
		},
		{
			title:       "No message index in error",
			err:         errors.New("some unrelated error"),
			expMsgIndex: 0,
			expFound:    false,
		},
		{
			title:       "Invalid message index (non-numeric)",
			err:         errors.New("message index: abc"),
			expMsgIndex: 0,
			expFound:    false,
		},
		{
			title:       "Empty error",
			err:         errors.New(""),
			expMsgIndex: 0,
			expFound:    false,
		},
		{
			title:       "Multiple message indices (uses first)",
			err:         errors.New("failed op: message index: 4; message index: 9"),
			expMsgIndex: 4,
			expFound:    true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()

			msgIndex, found := clientcontroller.FailedMessageIndex(tc.err)
			require.Equal(t, tc.expMsgIndex, msgIndex)
			require.Equal(t, tc.expFound, found)
		})
	}
}

func TestRemoveMsgAtIndex(t *testing.T) {
	t.Parallel()
	mockMsg := func(id string) sdk.Msg {
		return testdata.NewTestMsg(sdk.AccAddress(id))
	}

	tests := []struct {
		title         string
		msgs          []sdk.Msg
		indexToRemove int
		expectedMsgs  []sdk.Msg
	}{
		{
			title:         "Remove first message",
			msgs:          []sdk.Msg{mockMsg("1"), mockMsg("2"), mockMsg("3")},
			indexToRemove: 0,
			expectedMsgs:  []sdk.Msg{mockMsg("2"), mockMsg("3")},
		},
		{
			title:         "Remove middle message",
			msgs:          []sdk.Msg{mockMsg("1"), mockMsg("2"), mockMsg("3")},
			indexToRemove: 1,
			expectedMsgs:  []sdk.Msg{mockMsg("1"), mockMsg("3")},
		},
		{
			title:         "Remove last message",
			msgs:          []sdk.Msg{mockMsg("1"), mockMsg("2"), mockMsg("3")},
			indexToRemove: 2,
			expectedMsgs:  []sdk.Msg{mockMsg("1"), mockMsg("2")},
		},
		{
			title:         "Out-of-bounds index (negative)",
			msgs:          []sdk.Msg{mockMsg("1"), mockMsg("2"), mockMsg("3")},
			indexToRemove: -1,
			expectedMsgs:  []sdk.Msg{mockMsg("1"), mockMsg("2"), mockMsg("3")}, // No change
		},
		{
			title:         "Out-of-bounds index (too large)",
			msgs:          []sdk.Msg{mockMsg("1"), mockMsg("2"), mockMsg("3")},
			indexToRemove: 5,
			expectedMsgs:  []sdk.Msg{mockMsg("1"), mockMsg("2"), mockMsg("3")}, // No change
		},
		{
			title:         "Empty slice",
			msgs:          []sdk.Msg{},
			indexToRemove: 0,
			expectedMsgs:  []sdk.Msg{}, // No change
		},
		{
			title:         "Single element slice",
			msgs:          []sdk.Msg{mockMsg("1")},
			indexToRemove: 0,
			expectedMsgs:  []sdk.Msg{}, // Successfully removes the only element
		},
	}

	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()
			result := clientcontroller.RemoveMsgAtIndex(tc.msgs, tc.indexToRemove)

			require.Equal(t, len(tc.expectedMsgs), len(result))
			for i, resultMsg := range result {
				require.Equal(t, tc.expectedMsgs[i].String(), resultMsg.String())
			}
		})
	}
}
