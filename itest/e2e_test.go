//go:build e2e
// +build e2e

package e2etest

import (
	"testing"
	"time"

	"github.com/babylonlabs-io/covenant-emulator/clientcontroller"
	"github.com/babylonlabs-io/covenant-emulator/types"
	"github.com/stretchr/testify/require"
)

var (
	stakingTime   = uint16(100)
	stakingAmount = int64(20000)
)

func TestCovenantEmulatorLifeCycle(t *testing.T) {
	tm, btcPks := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	// send a BTC delegation that is not following pre-approval flow
	_ = tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)

	// check the BTC delegation is pending
	_ = tm.WaitForNPendingDels(t, 1)

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// send a BTC delegation that is following pre-approval flow
	_ = tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, true)

	// check the BTC delegation is pending
	_ = tm.WaitForNPendingDels(t, 1)

	time.Sleep(10 * time.Second)

	// check the BTC delegation is verified
	dels := tm.WaitForNVerifiedDels(t, 1)

	// test duplicate, should expect no error
	// remove covenant sigs
	dels[0].CovenantSigs = nil
	dels[0].BtcUndelegation.CovenantSlashingSigs = nil
	dels[0].BtcUndelegation.CovenantUnbondingSigs = nil
	res, err := tm.CovenantEmulator.AddCovenantSignatures(dels)
	require.NoError(t, err)
	require.Empty(t, res)
}

func TestQueryPendingDelegations(t *testing.T) {
	tm, btcPks := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	// manually sets the pg to a low value
	clientcontroller.MaxPaginationLimit = 2

	numDels := 3
	for i := 0; i < numDels; i++ {
		_ = tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)
	}

	dels, err := tm.CovBBNClient.QueryPendingDelegations(uint64(numDels), nil)
	require.NoError(t, err)
	require.Len(t, dels, numDels)
}

func TestSubmitCovenantSigsWithRetry(t *testing.T) {
	tm, btcPks := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	delData1 := tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)
	_ = tm.WaitForNPendingDels(t, 1)

	delData2 := tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)
	_ = tm.WaitForNPendingDels(t, 2)

	delData3 := tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)
	_ = tm.WaitForNPendingDels(t, 3)

	activeDels := tm.WaitForNActiveDels(t, 3)
	require.Len(t, activeDels, 3, "Expected 3 active delegations")

	findDelegation := func(txHash string) *types.Delegation {
		for _, d := range activeDels {
			if d.StakingTxHex == txHash {
				return d
			}
		}
		t.Fatalf("Could not find active delegation with staking tx hash %s", txHash)
		return nil
	}

	// Get the full delegation objects
	del1 := findDelegation(delData1.StakingTx.TxHash().String())
	del2 := findDelegation(delData2.StakingTx.TxHash().String())
	del3 := findDelegation(delData3.StakingTx.TxHash().String())

	txResp1, err := tm.CovenantEmulator.AddCovenantSignatures([]*types.Delegation{del1})
	require.NoError(t, err, "Failed to add covenant signatures for delegation 1")
	require.NotNil(t, txResp1, "Transaction response should not be nil for delegation 1")

	txResp2, err := tm.CovenantEmulator.AddCovenantSignatures([]*types.Delegation{del2})
	require.NoError(t, err, "Failed to add covenant signatures for delegation 2")
	require.NotNil(t, txResp2, "Transaction response should not be nil for delegation 2")

	// This should succeed with only del3 being processed (as del1 and del2 are already submitted)
	combinedDels := []*types.Delegation{del1, del2, del3}
	txRespCombined, err := tm.CovenantEmulator.AddCovenantSignatures(combinedDels)
	require.NoError(t, err, "Combined submission with duplicates failed unexpectedly")
	require.NotNil(t, txRespCombined, "Transaction response should not be nil on successful combined submission")

	require.NotEmpty(t, txRespCombined.TxHash, "Transaction hash should not be empty for combined submission")

	t.Logf("Successfully submitted batch with duplicates, TxHash: %s", txRespCombined.TxHash)
}
