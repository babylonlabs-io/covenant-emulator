//go:build e2e
// +build e2e

package e2etest

import (
	"testing"
	"time"

	"github.com/babylonlabs-io/covenant-emulator/clientcontroller"
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
