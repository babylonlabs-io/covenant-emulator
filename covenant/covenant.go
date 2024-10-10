package covenant

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon/btcstaking"
	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/covenant-emulator/clientcontroller"
	covcfg "github.com/babylonlabs-io/covenant-emulator/config"
	"github.com/babylonlabs-io/covenant-emulator/types"
)

var (
	// TODO: Maybe configurable?
	RtyAttNum = uint(5)
	RtyAtt    = retry.Attempts(RtyAttNum)
	RtyDel    = retry.Delay(time.Millisecond * 400)
	RtyErr    = retry.LastErrorOnly(true)
)

type CovenantEmulator struct {
	startOnce sync.Once
	stopOnce  sync.Once

	wg   sync.WaitGroup
	quit chan struct{}

	pk *btcec.PublicKey

	signer Signer
	cc     clientcontroller.ClientController

	config *covcfg.Config
	logger *zap.Logger
}

func NewCovenantEmulator(
	config *covcfg.Config,
	cc clientcontroller.ClientController,
	passphrase string,
	logger *zap.Logger,
	signer Signer,
) (*CovenantEmulator, error) {
	pk, err := signer.PubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get signer pub key: %w", err)
	}

	return &CovenantEmulator{
		cc:     cc,
		signer: signer,
		config: config,
		logger: logger,
		pk:     pk,
		quit:   make(chan struct{}),
	}, nil
}

func (ce *CovenantEmulator) Config() *covcfg.Config {
	return ce.config
}

func (ce *CovenantEmulator) PublicKeyStr() string {
	return hex.EncodeToString(schnorr.SerializePubKey(ce.pk))
}

// AddCovenantSignatures adds Covenant signatures on every given Bitcoin delegations and submits them
// in a batch to Babylon. Invalid delegations will be skipped with error log error will be returned if
// the batch submission fails
func (ce *CovenantEmulator) AddCovenantSignatures(btcDels []*types.Delegation) (*types.TxResponse, error) {
	if len(btcDels) == 0 {
		return nil, fmt.Errorf("no delegations")
	}

	signingReq := make(map[chainhash.Hash]SigningTxsRequest, len(btcDels))
	for _, btcDel := range btcDels {
		// 0. nil checks
		if btcDel == nil {
			ce.logger.Error("empty delegation")
			continue
		}

		if btcDel.BtcUndelegation == nil {
			ce.logger.Error("empty undelegation",
				zap.String("staking_tx_hex", btcDel.StakingTxHex))
			continue
		}

		// 1. get the params matched to the delegation version
		params, err := ce.getParamsByVersionWithRetry(btcDel.ParamsVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to get staking params with version %d: %w", btcDel.ParamsVersion, err)
		}

		// 2. the quorum is already achieved, skip sending more sigs
		stakerPkHex := hex.EncodeToString(schnorr.SerializePubKey(btcDel.BtcPk))
		if btcDel.HasCovenantQuorum(params.CovenantQuorum) {
			ce.logger.Error("covenant signatures already fulfilled",
				zap.String("staker_pk", stakerPkHex),
				zap.String("staking_tx_hex", btcDel.StakingTxHex),
			)
			continue
		}

		// 3. check unbonding time (staking time from unbonding tx) is larger than min unbonding time
		// which is larger value from:
		// - MinUnbondingTime
		// - CheckpointFinalizationTimeout
		unbondingTime := btcDel.UnbondingTime
		minUnbondingTime := params.MinimumUnbondingTime()
		if uint64(unbondingTime) <= minUnbondingTime {
			ce.logger.Error("invalid unbonding time",
				zap.Uint64("min_unbonding_time", minUnbondingTime),
				zap.Uint16("got_unbonding_time", unbondingTime),
			)
			continue
		}

		// 4. check whether the:
		// - staking time is within the min and max staking time
		// - staking value is within the min and max staking value
		stakingTime := btcDel.GetStakingTime()

		if stakingTime < params.MinStakingTime || stakingTime > params.MaxStakingTime {
			ce.logger.Error("invalid staking time",
				zap.Uint16("min_staking_time", params.MinStakingTime),
				zap.Uint16("max_staking_time", params.MaxStakingTime),
				zap.Uint16("got_staking_time", stakingTime),
			)
			continue
		}

		if btcDel.TotalSat < params.MinStakingValue || btcDel.TotalSat > params.MaxStakingValue {
			ce.logger.Error("invalid staking value",
				zap.Int64("min_staking_value", int64(params.MinStakingValue)),
				zap.Int64("max_staking_value", int64(params.MaxStakingValue)),
				zap.Int64("got_staking_value", int64(btcDel.TotalSat)),
			)
			continue
		}

		// 5. decode staking tx and slashing tx from the delegation
		stakingTx, slashingTx, err := decodeDelegationTransactions(btcDel, params, &ce.config.BTCNetParams)
		if err != nil {
			ce.logger.Error("invalid delegation",
				zap.String("staker_pk", stakerPkHex),
				zap.String("staking_tx_hex", btcDel.StakingTxHex),
				zap.String("slashing_tx_hex", btcDel.SlashingTxHex),
				zap.Error(err),
			)
			continue
		}

		// 6. decode unbonding tx and slash unbonding tx from the undelegation
		unbondingTx, slashUnbondingTx, err := decodeUndelegationTransactions(btcDel, params, &ce.config.BTCNetParams)
		if err != nil {
			ce.logger.Error("invalid undelegation",
				zap.String("staker_pk", stakerPkHex),
				zap.String("unbonding_tx_hex", btcDel.BtcUndelegation.UnbondingTxHex),
				zap.String("unbonding_slashing_tx_hex", btcDel.BtcUndelegation.SlashingTxHex),
				zap.Error(err),
			)
			continue
		}

		// 7. Check unbonding fee
		unbondingFee := stakingTx.TxOut[btcDel.StakingOutputIdx].Value - unbondingTx.TxOut[0].Value
		if unbondingFee != int64(params.UnbondingFee) {
			ce.logger.Error("invalid unbonding fee",
				zap.Int64("expected_unbonding_fee", int64(params.UnbondingFee)),
				zap.Int64("got_unbonding_fee", unbondingFee),
			)
			continue
		}

		// 8. Generate Signing Request
		// Finality providers encription keys
		// pk script paths for Slash, unbond and unbonding slashing
		fpsEncKeys, err := fpEncKeysFromDel(btcDel)
		if err != nil {
			ce.logger.Error("failed to encript the finality provider keys of the btc delegation", zap.String("staker_pk", stakerPkHex), zap.Error(err))
			continue
		}

		slashingPkScriptPath, stakingTxUnbondingPkScriptPath, unbondingTxSlashingPkScriptPath, err := pkScriptPaths(btcDel, params, &ce.config.BTCNetParams, unbondingTx)
		if err != nil {
			ce.logger.Error("failed to generate pk script path", zap.Error(err))
			continue
		}

		signingReq[stakingTx.TxHash()] = SigningTxsRequest{
			StakingTx:                       stakingTx,
			SlashingTx:                      slashingTx,
			UnbondingTx:                     unbondingTx,
			SlashUnbondingTx:                slashUnbondingTx,
			StakingOutputIdx:                btcDel.StakingOutputIdx,
			SlashingPkScriptPath:            slashingPkScriptPath,
			StakingTxUnbondingPkScriptPath:  stakingTxUnbondingPkScriptPath,
			UnbondingTxSlashingPkScriptPath: unbondingTxSlashingPkScriptPath,
			FpEncKeys:                       fpsEncKeys,
		}
	}

	respSigs, err := ce.SignTransactions(signingReq)
	if err != nil {
		return nil, err
	}

	covenantSigs := BuildCovenantSigs(ce.pk, *respSigs)

	// 9. submit covenant sigs
	res, err := ce.cc.SubmitCovenantSigs(covenantSigs)
	if err != nil {
		ce.recordMetricsFailedSignDelegations(len(covenantSigs))
		return nil, err
	}

	// record metrics
	submittedTime := time.Now()
	metricsTimeKeeper.SetPreviousSubmission(&submittedTime)
	ce.recordMetricsTotalSignDelegationsSubmitted(len(covenantSigs))

	return res, nil
}

// BuildCovenantSigs creates the covenant signatures from the signature response
func BuildCovenantSigs(pk *secp.PublicKey, resp SigningResponse) []*types.CovenantSigs {
	covenantSigs := make([]*types.CovenantSigs, 0, len(resp.SignaturesByStkTxHash))
	for stkTxHash, signatures := range resp.SignaturesByStkTxHash {
		covenantSigs = append(covenantSigs, &types.CovenantSigs{
			PublicKey:             pk,
			StakingTxHash:         stkTxHash,
			SlashingSigs:          signatures.SlashSigs,
			UnbondingSig:          signatures.UnbondingSig,
			SlashingUnbondingSigs: signatures.SlashUnbondingSigs,
		})
	}
	return SortCovenantSigs(covenantSigs)
}

// SignTransactions calls the signer and record metrics about signing
func (ce *CovenantEmulator) SignTransactions(signingReq map[chainhash.Hash]SigningTxsRequest) (*SigningResponse, error) {
	// record metrics
	startSignTime := time.Now()
	metricsTimeKeeper.SetPreviousSignStart(&startSignTime)

	// 8. sign covenant transactions
	respSignatures, err := ce.signer.SignTransactions(SigningRequest{SigningTxsReqByStkTxHash: signingReq})
	if err != nil {
		ce.recordMetricsFailedSignDelegations(len(signingReq))
		return nil, err
	}

	// record metrics
	finishSignTime := time.Now()
	metricsTimeKeeper.SetPreviousSignFinish(&finishSignTime)
	timedSignDelegationLag.Observe(time.Since(startSignTime).Seconds())

	return respSignatures, nil
}

func fpEncKeysFromDel(btcDel *types.Delegation) ([]*asig.EncryptionKey, error) {
	fpsEncKeys := make([]*asig.EncryptionKey, 0, len(btcDel.FpBtcPks))
	for _, fpPk := range btcDel.FpBtcPks {
		encKey, err := asig.NewEncryptionKeyFromBTCPK(fpPk)
		if err != nil {
			fpPkHex := bbntypes.NewBIP340PubKeyFromBTCPK(fpPk).MarshalHex()
			return nil, fmt.Errorf("failed to get encryption key from finality provider public key %s: %w", fpPkHex, err)
		}
		fpsEncKeys = append(fpsEncKeys, encKey)
	}

	return fpsEncKeys, nil
}

func pkScriptPathUnbondingSlash(
	del *types.Delegation,
	unbondingTx *wire.MsgTx,
	params *types.StakingParams,
	btcNet *chaincfg.Params,
) (unbondingTxSlashingScriptPath []byte, err error) {
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		del.BtcPk,
		del.FpBtcPks,
		params.CovenantPks,
		params.CovenantQuorum,
		del.UnbondingTime,
		btcutil.Amount(unbondingTx.TxOut[0].Value),
		btcNet,
	)
	if err != nil {
		return nil, err
	}

	unbondingTxSlashingPathInfo, err := unbondingInfo.SlashingPathSpendInfo()
	if err != nil {
		return nil, err
	}
	unbondingTxSlashingScriptPath = unbondingTxSlashingPathInfo.GetPkScriptPath()

	return unbondingTxSlashingScriptPath, nil
}

func pkScriptPaths(
	del *types.Delegation,
	params *types.StakingParams,
	btcNet *chaincfg.Params,
	unbondingTx *wire.MsgTx,
) (slash, unbond, unbondSlash []byte, err error) {
	slash, unbond, err = pkScriptPathSlashAndUnbond(del, params, btcNet)
	if err != nil {
		return nil, nil, nil, err
	}

	unbondSlash, err = pkScriptPathUnbondingSlash(del, unbondingTx, params, btcNet)
	if err != nil {
		return nil, nil, nil, err
	}

	return slash, unbond, unbondSlash, nil
}

func pkScriptPathSlashAndUnbond(
	del *types.Delegation,
	params *types.StakingParams,
	btcNet *chaincfg.Params,
) (slashingPkScriptPath, stakingTxUnbondingPkScriptPath []byte, err error) {
	// sign slash signatures with every finality providers
	stakingInfo, err := btcstaking.BuildStakingInfo(
		del.BtcPk,
		del.FpBtcPks,
		params.CovenantPks,
		params.CovenantQuorum,
		del.GetStakingTime(),
		del.TotalSat,
		btcNet,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build staking info: %w", err)
	}

	slashingPathInfo, err := stakingInfo.SlashingPathSpendInfo()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get slashing path info: %w", err)
	}
	slashingPkScriptPath = slashingPathInfo.GetPkScriptPath()

	// sign unbonding sig
	stakingTxUnbondingPathInfo, err := stakingInfo.UnbondingPathSpendInfo()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get unbonding path spend info")
	}
	stakingTxUnbondingPkScriptPath = stakingTxUnbondingPathInfo.GetPkScriptPath()

	return slashingPkScriptPath, stakingTxUnbondingPkScriptPath, nil
}

func decodeDelegationTransactions(del *types.Delegation, params *types.StakingParams, btcNet *chaincfg.Params) (*wire.MsgTx, *wire.MsgTx, error) {
	// 1. decode staking tx and slashing tx
	stakingMsgTx, _, err := bbntypes.NewBTCTxFromHex(del.StakingTxHex)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode staking tx from hex")
	}

	slashingTx, err := bstypes.NewBTCSlashingTxFromHex(del.SlashingTxHex)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode slashing tx from hex")
	}

	slashingMsgTx, err := slashingTx.ToMsgTx()
	if err != nil {
		return nil, nil, err
	}

	// 2. verify the transactions
	if err := btcstaking.CheckTransactions(
		slashingMsgTx,
		stakingMsgTx,
		del.StakingOutputIdx,
		int64(params.MinSlashingTxFeeSat),
		params.SlashingRate,
		params.SlashingPkScript,
		del.BtcPk,
		del.UnbondingTime,
		btcNet,
	); err != nil {
		return nil, nil, fmt.Errorf("invalid txs in the delegation: %w", err)
	}

	return stakingMsgTx, slashingMsgTx, nil
}

func decodeUndelegationTransactions(del *types.Delegation, params *types.StakingParams, btcNet *chaincfg.Params) (*wire.MsgTx, *wire.MsgTx, error) {
	// 1. decode unbonding tx and slashing tx
	unbondingMsgTx, _, err := bbntypes.NewBTCTxFromHex(del.BtcUndelegation.UnbondingTxHex)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode unbonding tx from hex: %w", err)
	}

	unbondingSlashingMsgTx, _, err := bbntypes.NewBTCTxFromHex(del.BtcUndelegation.SlashingTxHex)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode unbonding slashing tx from hex: %w", err)
	}

	// 2. verify transactions
	if err := btcstaking.CheckTransactions(
		unbondingSlashingMsgTx,
		unbondingMsgTx,
		0,
		int64(params.MinSlashingTxFeeSat),
		params.SlashingRate,
		params.SlashingPkScript,
		del.BtcPk,
		del.UnbondingTime,
		btcNet,
	); err != nil {
		return nil, nil, fmt.Errorf("invalid txs in the undelegation: %w", err)
	}

	return unbondingMsgTx, unbondingSlashingMsgTx, err
}

// delegationsToBatches takes a list of delegations and splits them into batches
func (ce *CovenantEmulator) delegationsToBatches(dels []*types.Delegation) [][]*types.Delegation {
	batchSize := ce.config.SigsBatchSize
	batches := make([][]*types.Delegation, 0)

	for i := uint64(0); i < uint64(len(dels)); i += batchSize {
		end := i + batchSize
		if end > uint64(len(dels)) {
			end = uint64(len(dels))
		}
		batches = append(batches, dels[i:end])
	}

	return batches
}

// removeAlreadySigned removes any delegations that have already been signed by the covenant
func (ce *CovenantEmulator) removeAlreadySigned(dels []*types.Delegation) []*types.Delegation {
	sanitized := make([]*types.Delegation, 0, len(dels))

	for _, del := range dels {
		delCopy := del
		alreadySigned := false
		for _, covSig := range delCopy.CovenantSigs {
			if covSig.Pk.IsEqual(ce.pk) {
				alreadySigned = true
				break
			}
		}
		if !alreadySigned {
			sanitized = append(sanitized, delCopy)
		}
	}
	return sanitized
}

// covenantSigSubmissionLoop is the reactor to submit Covenant signature for BTC delegations
func (ce *CovenantEmulator) covenantSigSubmissionLoop() {
	defer ce.wg.Done()

	interval := ce.config.QueryInterval
	limit := ce.config.DelegationLimit
	covenantSigTicker := time.NewTicker(interval)

	ce.logger.Info("starting signature submission loop",
		zap.Float64("interval seconds", interval.Seconds()))

	for {
		select {
		case <-covenantSigTicker.C:
			// 1. Get all pending delegations
			dels, err := ce.cc.QueryPendingDelegations(limit)
			if err != nil {
				ce.logger.Debug("failed to get pending delegations", zap.Error(err))
				continue
			}

			// record delegation metrics
			ce.recordMetricsCurrentPendingDelegations(len(dels))

			if len(dels) == 0 {
				ce.logger.Debug("no pending delegations are found")
			}
			// 2. Remove delegations that do not need the covenant's signature
			sanitizedDels := ce.removeAlreadySigned(dels)

			// 3. Split delegations into batches for submission
			batches := ce.delegationsToBatches(sanitizedDels)
			for _, delBatch := range batches {
				_, err := ce.AddCovenantSignatures(delBatch)
				if err != nil {
					ce.logger.Error(
						"failed to submit covenant signatures for BTC delegations",
						zap.Error(err),
					)
				}
			}

		case <-ce.quit:
			ce.logger.Debug("exiting covenant signature submission loop")
			return
		}
	}

}

func (ce *CovenantEmulator) metricsUpdateLoop() {
	defer ce.wg.Done()

	interval := ce.config.Metrics.UpdateInterval
	ce.logger.Info("starting metrics update loop",
		zap.Float64("interval seconds", interval.Seconds()))
	updateTicker := time.NewTicker(interval)

	for {
		select {
		case <-updateTicker.C:
			metricsTimeKeeper.UpdatePrometheusMetrics()
		case <-ce.quit:
			updateTicker.Stop()
			ce.logger.Info("exiting metrics update loop")
			return
		}
	}
}

func (ce *CovenantEmulator) getParamsByVersionWithRetry(version uint32) (*types.StakingParams, error) {
	var (
		params *types.StakingParams
		err    error
	)

	if err := retry.Do(func() error {
		params, err = ce.cc.QueryStakingParamsByVersion(version)
		if err != nil {
			return err
		}
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		ce.logger.Debug(
			"failed to query the consumer chain for the staking params",
			zap.Uint("attempt", n+1),
			zap.Uint("max_attempts", RtyAttNum),
			zap.Error(err),
		)
	})); err != nil {
		return nil, err
	}

	return params, nil
}

func (ce *CovenantEmulator) recordMetricsFailedSignDelegations(n int) {
	failedSignDelegations.WithLabelValues(ce.PublicKeyStr()).Add(float64(n))
}

func (ce *CovenantEmulator) recordMetricsTotalSignDelegationsSubmitted(n int) {
	totalSignDelegationsSubmitted.WithLabelValues(ce.PublicKeyStr()).Add(float64(n))
}

func (ce *CovenantEmulator) recordMetricsCurrentPendingDelegations(n int) {
	currentPendingDelegations.WithLabelValues(ce.PublicKeyStr()).Set(float64(n))
}

func (ce *CovenantEmulator) Start() error {
	var startErr error
	ce.startOnce.Do(func() {
		ce.logger.Info("Starting Covenant Emulator")

		ce.wg.Add(2)
		go ce.covenantSigSubmissionLoop()
		go ce.metricsUpdateLoop()
	})

	return startErr
}

func (ce *CovenantEmulator) Stop() error {
	var stopErr error
	ce.stopOnce.Do(func() {
		ce.logger.Info("Stopping Covenant Emulator")

		close(ce.quit)
		ce.wg.Wait()

		ce.logger.Debug("Covenant Emulator successfully stopped")
	})
	return stopErr
}

// SortCovenantSigs helper function to sort all covenant signatures by the staking tx hash
func SortCovenantSigs(covSigs []*types.CovenantSigs) []*types.CovenantSigs {
	sorted := make([]*types.CovenantSigs, len(covSigs))
	copy(sorted, covSigs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.Compare(sorted[i].StakingTxHash.String(), sorted[j].StakingTxHash.String()) == 1
	})

	return sorted
}
