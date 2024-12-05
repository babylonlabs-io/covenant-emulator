package testutil

import (
	"encoding/hex"
	"math"
	"math/rand"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/covenant-emulator/types"
)

type TestStakingSlashingInfo struct {
	StakingTx   *wire.MsgTx
	SlashingTx  *bstypes.BTCSlashingTx
	StakingInfo *btcstaking.StakingInfo
}

type spendableOut struct {
	prevOut wire.OutPoint
	amount  btcutil.Amount
}

func GenRandomHexStr(r *rand.Rand, length uint64) string {
	randBytes := datagen.GenRandomByteArray(r, length)
	return hex.EncodeToString(randBytes)
}

func AddRandomSeedsToFuzzer(f *testing.F, num uint) {
	// Seed based on the current time
	r := rand.New(rand.NewSource(time.Now().Unix()))
	var idx uint
	for idx = 0; idx < num; idx++ {
		f.Add(r.Int63())
	}
}

func GenValidSlashingRate(r *rand.Rand) sdkmath.LegacyDec {
	return sdkmath.LegacyNewDecWithPrec(int64(datagen.RandomInt(r, 41)+10), 2)
}

func RandRange(r *rand.Rand, min, max int) int {
	return rand.Intn(max+1-min) + min
}

func GenRandomParams(r *rand.Rand, t *testing.T) *types.StakingParams {
	covThreshold := datagen.RandomInt(r, 5) + 1
	covNum := covThreshold * 2
	covenantPks := make([]*btcec.PublicKey, 0, covNum)
	for i := 0; i < int(covNum); i++ {
		_, covPk, err := datagen.GenRandomBTCKeyPair(r)
		require.NoError(t, err)
		covenantPks = append(covenantPks, covPk)
	}

	slashingAddr, err := datagen.GenRandomBTCAddress(r, &chaincfg.SimNetParams)
	require.NoError(t, err)
	slashingPkScript, err := txscript.PayToAddrScript(slashingAddr)
	require.NoError(t, err)

	return &types.StakingParams{
		ComfirmationTimeBlocks:    10,
		FinalizationTimeoutBlocks: 100,
		UnbondingTimeBlocks:       101,
		MinSlashingTxFeeSat:       1,
		CovenantPks:               covenantPks,
		SlashingPkScript:          slashingPkScript,
		CovenantQuorum:            uint32(covThreshold),
		SlashingRate:              GenValidSlashingRate(r),
		UnbondingFee:              btcutil.Amount(1000),
		MinStakingTime:            10,
		MaxStakingTime:            math.MaxUint16,
		MinStakingValue:           btcutil.Amount(10000),
		MaxStakingValue:           btcutil.Amount(10 * 10e8),
	}
}

func GenBtcPublicKeys(r *rand.Rand, t *testing.T, num int) []*btcec.PublicKey {
	pks := make([]*btcec.PublicKey, 0, num)
	for i := 0; i < num; i++ {
		_, covPk, err := datagen.GenRandomBTCKeyPair(r)
		require.NoError(t, err)
		pks = append(pks, covPk)
	}

	return pks
}

func GenBTCStakingSlashingInfo(
	r *rand.Rand,
	t testing.TB,
	btcNet *chaincfg.Params,
	stakerSK *btcec.PrivateKey,
	fpPKs []*btcec.PublicKey,
	covenantPKs []*btcec.PublicKey,
	covenantQuorum uint32,
	stakingTimeBlocks uint16,
	stakingValue int64,
	slashingPkScript []byte,
	slashingRate sdkmath.LegacyDec,
	slashingChangeLockTime uint16,
) *TestStakingSlashingInfo {
	// an arbitrary input
	unbondingTxFee := r.Int63n(1000) + 1
	spend := makeSpendableOutWithRandOutPoint(r, btcutil.Amount(stakingValue+unbondingTxFee))
	outPoint := &spend.prevOut
	return GenBTCStakingSlashingInfoWithOutPoint(
		r,
		t,
		btcNet,
		outPoint,
		stakerSK,
		fpPKs,
		covenantPKs,
		covenantQuorum,
		stakingTimeBlocks,
		stakingValue,
		slashingPkScript,
		slashingRate,
		slashingChangeLockTime,
	)
}

func makeSpendableOutWithRandOutPoint(r *rand.Rand, amount btcutil.Amount) spendableOut {
	out := randOutPoint(r)

	return spendableOut{
		prevOut: out,
		amount:  amount,
	}
}

func GenBTCStakingSlashingInfoWithOutPoint(
	r *rand.Rand,
	t testing.TB,
	btcNet *chaincfg.Params,
	outPoint *wire.OutPoint,
	stakerSK *btcec.PrivateKey,
	fpPKs []*btcec.PublicKey,
	covenantPKs []*btcec.PublicKey,
	covenantQuorum uint32,
	stakingTimeBlocks uint16,
	stakingValue int64,
	slashingPkScript []byte,
	slashingRate sdkmath.LegacyDec,
	slashingChangeLockTime uint16,
) *TestStakingSlashingInfo {

	stakingInfo, err := btcstaking.BuildStakingInfo(
		stakerSK.PubKey(),
		fpPKs,
		covenantPKs,
		covenantQuorum,
		stakingTimeBlocks,
		btcutil.Amount(stakingValue),
		btcNet,
	)

	require.NoError(t, err)
	tx := wire.NewMsgTx(2)

	// 2 outputs for changes and staking output
	changeAddrScript, err := datagen.GenRandomPubKeyHashScript(r, btcNet)
	require.NoError(t, err)
	require.False(t, txscript.GetScriptClass(changeAddrScript) == txscript.NonStandardTy)

	tx.AddTxOut(wire.NewTxOut(10000, changeAddrScript)) // output for change

	// add the given tx input
	txIn := wire.NewTxIn(outPoint, nil, nil)
	tx.AddTxIn(txIn)
	tx.AddTxOut(stakingInfo.StakingOutput)

	slashingMsgTx, err := btcstaking.BuildSlashingTxFromStakingTxStrict(
		tx,
		uint32(1),
		slashingPkScript,
		stakerSK.PubKey(),
		slashingChangeLockTime,
		2000,
		slashingRate,
		btcNet)
	require.NoError(t, err)
	slashingTx, err := bstypes.NewBTCSlashingTxFromMsgTx(slashingMsgTx)
	require.NoError(t, err)

	return &TestStakingSlashingInfo{
		StakingTx:   tx,
		SlashingTx:  slashingTx,
		StakingInfo: stakingInfo,
	}
}

func randOutPoint(r *rand.Rand) wire.OutPoint {
	hash, _ := chainhash.NewHash(datagen.GenRandomByteArray(r, chainhash.HashSize))
	// TODO this will be deterministic without seed but for now it is not that
	// important
	idx := r.Uint32()

	return wire.OutPoint{
		Hash:  *hash,
		Index: idx,
	}
}
