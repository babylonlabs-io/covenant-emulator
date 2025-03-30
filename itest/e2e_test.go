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

	delData := tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)
	_ = tm.WaitForNPendingDels(t, 1)
	_ = tm.WaitForNActiveDels(t, 1)

	time.Sleep(10 * time.Second)

	dels := tm.WaitForNVerifiedDels(t, 1)

	// Create two invalid delegations
	invalidDel1 := &types.Delegation{
		StakingTxHex: "invalid_hex",
		BtcUndelegation: &types.Undelegation{
			UnbondingTxHex: "invalid_unbonding_hex",
			SlashingTxHex:  "invalid_slashing_hex",
		},
	}

	invalidDel2 := &types.Delegation{
		StakingTxHex: "another_invalid_hex",
	}

	// Submit a batch with 2 invalid delegations and 1 valid
	// The retry logic should process only the valid one
	combinedDels := []*types.Delegation{invalidDel1, invalidDel2, dels[0]}
	txRespCombined, err := tm.CovenantEmulator.AddCovenantSignatures(combinedDels)
	require.NoError(t, err, "Combined submission with mixed valid/invalid delegations failed unexpectedly")
	require.NotNil(t, txRespCombined, "Transaction response should not be nil on successful combined submission")
	require.NotEmpty(t, txRespCombined.TxHash, "Transaction hash should not be empty for combined submission")

	t.Logf("Successfully submitted batch with mixed valid/invalid delegations, TxHash: %s", txRespCombined.TxHash)
}
