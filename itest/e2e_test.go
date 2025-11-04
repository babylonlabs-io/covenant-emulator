//go:build e2e

package e2etest

import (
	"testing"
	"time"

	"github.com/babylonlabs-io/covenant-emulator/clientcontroller"
	"github.com/babylonlabs-io/covenant-emulator/covenant"
	"github.com/babylonlabs-io/covenant-emulator/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
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

func TestStakeExpansionDelegation(t *testing.T) {
	tm, btcPks := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	// First, create a regular BTC delegation that will be used as the base for expansion
	baseDel := tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)

	// Wait for the base delegation to become active
	activeDels := tm.WaitForNActiveDels(t, 1)
	require.Len(t, activeDels, 1)

	// Create a stake expansion delegation using the base delegation's staking tx hash
	_ = tm.InsertStakeExpansionDelegation(t, btcPks, stakingTime, stakingAmount, baseDel.StakingTx, true)

	// Wait for the stake expansion delegation to become pending
	pendingDels := tm.WaitForNPendingDels(t, 1)
	require.Len(t, pendingDels, 1)

	// Wait for the stake expansion delegation to become verified after getting covenant signatures
	verifiedDels := tm.WaitForNVerifiedDels(t, 1)
	require.Len(t, verifiedDels, 1)
	require.NotNil(t, verifiedDels[0].StakeExpansion)

	t.Log("stake expansion delegation test completed successfully")
}

func TestSubmitCovenantSigsBatchToSubmission(t *testing.T) {
	tm, btcPks := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	// First delegation
	tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, true)
	_ = tm.WaitForNPendingDels(t, 1)
	time.Sleep(5 * time.Second)
	dels1 := tm.WaitForNVerifiedDels(t, 1)

	// Second delegation
	tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, true)
	_ = tm.WaitForNPendingDels(t, 1)
	time.Sleep(5 * time.Second)
	dels2 := tm.WaitForNVerifiedDels(t, 2)

	// we stop the emulator to make sure we don't process del3
	err := tm.CovenantEmulator.Stop()
	require.NoError(t, err)

	// Third delegation
	tm.InsertBTCDelegation(t, btcPks, stakingTime, stakingAmount, true)
	dels3 := tm.WaitForNPendingDels(t, 1)

	del1 := dels1[0]
	del1.CovenantSigs = nil
	del1.BtcUndelegation.CovenantSlashingSigs = nil
	del1.BtcUndelegation.CovenantUnbondingSigs = nil

	del2 := dels2[1]
	del2.CovenantSigs = nil
	del2.BtcUndelegation.CovenantSlashingSigs = nil
	del2.BtcUndelegation.CovenantUnbondingSigs = nil

	del3 := dels3[0]

	batchDels := []*types.Delegation{del1, del2, del3}
	require.NoError(t, err)

	// we reinitialize the emulator but don't start it
	bbnCfg := defaultBBNConfigWithKey("test-spending-key", tm.BabylonHandler.GetNodeDataDir())
	covbc, err := clientcontroller.NewBabylonController(bbnCfg, &tm.CovenanConfig.BTCNetParams,
		zaptest.NewLogger(t), tm.CovenanConfig.MaxRetiresBatchRemovingMsgs)
	require.NoError(t, err)
	ce, err := covenant.NewEmulator(tm.CovenanConfig, covbc, zaptest.NewLogger(t), tm.Signer)
	require.NoError(t, err)

	txRespBatch, err := ce.AddCovenantSignatures(batchDels)
	require.NoError(t, err, "Batch submission should succeed")
	require.NotNil(t, txRespBatch, "Transaction response should not be nil")
	require.NotEmpty(t, txRespBatch.TxHash, "Transaction hash should not be empty")
	t.Logf("Successfully submitted batch of %d delegations, TxHash: %s", len(batchDels), txRespBatch.TxHash)
}

func TestMultisigBtcDelegation(t *testing.T) {
	tm, btcPks := StartManagerWithFinalityProvider(t, 1)
	defer tm.Stop(t)

	// send a BTC delegation that is not following pre-approval flow
	_ = tm.InsertMultisigBTCDelegation(t, btcPks, stakingTime, stakingAmount, false)

	// check the BTC delegation is pending
	_ = tm.WaitForNPendingDels(t, 1)

	// check the BTC delegation is active
	_ = tm.WaitForNActiveDels(t, 1)

	// send a BTC delegation that is following pre-approval flow
	_ = tm.InsertMultisigBTCDelegation(t, btcPks, stakingTime, stakingAmount, true)

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
