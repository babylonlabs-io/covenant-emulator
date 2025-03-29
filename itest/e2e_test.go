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
	_ = tm.WaitForNActiveDels(t, 1)
	activeDels1, err := tm.CovBBNClient.QueryActiveDelegations(100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(activeDels1), 1, "Expected at least 1 active delegation")

	delData2 := tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)
	_ = tm.WaitForNPendingDels(t, 1)
	_ = tm.WaitForNActiveDels(t, 2)
	activeDels2, err := tm.CovBBNClient.QueryActiveDelegations(100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(activeDels2), 2, "Expected at least 2 active delegations")

	delData3 := tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)
	_ = tm.WaitForNPendingDels(t, 1)
	_ = tm.WaitForNActiveDels(t, 3)
	activeDels3, err := tm.CovBBNClient.QueryActiveDelegations(100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(activeDels3), 3, "Expected at least 3 active delegations")

	var del1, del2, del3 *types.Delegation

	for _, d := range activeDels3 {
		txHash := delData1.StakingTx.TxHash().String()
		if d.StakingTxHex == txHash {
			del1 = d
		}

		txHash = delData2.StakingTx.TxHash().String()
		if d.StakingTxHex == txHash {
			del2 = d
		}

		txHash = delData3.StakingTx.TxHash().String()
		if d.StakingTxHex == txHash {
			del3 = d
		}
	}

	require.NotNil(t, del1, "Could not find delegation 1")
	require.NotNil(t, del2, "Could not find delegation 2")
	require.NotNil(t, del3, "Could not find delegation 3")

	// Create invalid signature situation by clearing the signatures
	// This simulates having invalid signatures for del1 and del2
	del1.CovenantSigs = nil
	if del1.BtcUndelegation != nil {
		del1.BtcUndelegation.CovenantSlashingSigs = nil
		del1.BtcUndelegation.CovenantUnbondingSigs = nil
	}

	del2.CovenantSigs = nil
	if del2.BtcUndelegation != nil {
		del2.BtcUndelegation.CovenantSlashingSigs = nil
		del2.BtcUndelegation.CovenantUnbondingSigs = nil
	}

	// submit a batch with 2 invalid delegations and 1 valid
	// The retry logic should process only the valid one (del3)
	combinedDels := []*types.Delegation{del1, del2, del3}
	txRespCombined, err := tm.CovenantEmulator.AddCovenantSignatures(combinedDels)
	require.NoError(t, err, "Combined submission with mixed valid/invalid delegations failed unexpectedly")
	require.NotNil(t, txRespCombined, "Transaction response should not be nil on successful combined submission")
	require.NotEmpty(t, txRespCombined.TxHash, "Transaction hash should not be empty for combined submission")

	t.Logf("Successfully submitted batch with mixed valid/invalid delegations, TxHash: %s", txRespCombined.TxHash)
}
