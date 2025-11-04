package covenant

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon/v4/btcstaking"
	asig "github.com/babylonlabs-io/babylon/v4/crypto/schnorr-adaptor-signature"
	bbntypes "github.com/babylonlabs-io/babylon/v4/types"
	bstypes "github.com/babylonlabs-io/babylon/v4/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
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

type Emulator struct {
	startOnce sync.Once
	stopOnce  sync.Once

	wg   sync.WaitGroup
	quit chan struct{}

	pk *btcec.PublicKey

	signer Signer
	cc     clientcontroller.ClientController

	config *covcfg.Config
	logger *zap.Logger

	paramCache ParamsGetter
}

func NewEmulator(
	config *covcfg.Config,
	cc clientcontroller.ClientController,
	logger *zap.Logger,
	signer Signer,
) (*Emulator, error) {
	pk, err := signer.PubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get signer pub key: %w", err)
	}

	return &Emulator{
		cc:         cc,
		signer:     signer,
		config:     config,
		logger:     logger,
		pk:         pk,
		quit:       make(chan struct{}),
		paramCache: NewCacheVersionedParams(cc, logger),
	}, nil
}

func (ce *Emulator) Config() *covcfg.Config {
	return ce.config
}

func (ce *Emulator) PublicKeyStr() string {
	return hex.EncodeToString(schnorr.SerializePubKey(ce.pk))
}

// AddCovenantSignatures adds Covenant signatures on every given Bitcoin delegations and submits them
// in a batch to Babylon. Invalid delegations will be skipped with error log error will be returned if
// the batch submission fails
func (ce *Emulator) AddCovenantSignatures(btcDels []*types.Delegation) (*types.TxResponse, error) {
	if len(btcDels) == 0 {
		return nil, fmt.Errorf("no delegations")
	}

	covenantSigs := make([]*types.CovenantSigs, 0, len(btcDels))
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
		params, err := ce.paramCache.Get(btcDel.ParamsVersion)
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

		// 3. check unbonding time (staking time from unbonding tx) is larger or equal
		// to the minimum unbonding time in Babylon node parameters
		unbondingTime := btcDel.UnbondingTime
		unbondingTimeBlocks := params.UnbondingTimeBlocks
		if uint32(unbondingTime) != unbondingTimeBlocks {
			ce.logger.Error("invalid unbonding time",
				zap.Uint32("expected_unbonding_time", unbondingTimeBlocks),
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
		// Finality providers encryption keys
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

		req := SigningRequest{
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

		// 9. handle if it is stake expansion
		if btcDel.IsStakeExpansion() {
			stkExpReq, err := ce.buildStkExpSigningRequest(stakerPkHex, btcDel)
			if err != nil {
				ce.logger.Error("error building stake expansion request",
					zap.Error(err),
				)

				continue
			}

			req.StakeExp = stkExpReq
		}

		// 10. sign covenant transactions
		resp, err := ce.SignTransactions(req)
		if err != nil {
			ce.logger.Error("failed to sign transactions", zap.Error(err))

			continue
		}

		covenantSigs = append(covenantSigs, &types.CovenantSigs{
			PublicKey:             ce.pk,
			StakingTxHash:         stakingTx.TxHash(),
			SlashingSigs:          resp.SlashSigs,
			UnbondingSig:          resp.UnbondingSig,
			SlashingUnbondingSigs: resp.SlashUnbondingSigs,
			StkExpSig:             resp.StkExtSig,
		})
	}

	// 11. submit covenant sigs
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

func (ce *Emulator) buildStkExpSigningRequest(
	stakerPkHex string,
	btcDel *types.Delegation,
) (*SigningRequestStkExp, error) {
	if btcDel == nil || btcDel.StakeExpansion == nil {
		return nil, fmt.Errorf("stake expansion is nil in the delegation")
	}

	fundingTxOut, err := stakeExpValidateBasic(btcDel)
	if err != nil {
		ce.logger.Error("invalid stake expansion delegation",
			zap.String("staker_pk", stakerPkHex),
			zap.String("staking_tx_hex", btcDel.StakingTxHex),
			zap.String("stake_exp_tx_hash_hex", btcDel.StakeExpansion.PreviousStakingTxHashHex),
			zap.Error(err),
		)

		return nil, err
	}

	prevDel, err := ce.cc.QueryBTCDelegation(btcDel.StakeExpansion.PreviousStakingTxHashHex)
	if err != nil {
		ce.logger.Error("failed to query stake expansion", zap.Error(err), zap.String("stk_exp_tx_hash_hex", btcDel.StakeExpansion.PreviousStakingTxHashHex))

		return nil, err
	}

	previousActiveStkTx, _, err := bbntypes.NewBTCTxFromHex(prevDel.StakingTxHex)
	if err != nil {
		ce.logger.Error("failed to decode stake expansion tx", zap.Error(err), zap.String("stk_exp_tx_hash_hex", btcDel.StakeExpansion.PreviousStakingTxHashHex))

		return nil, err
	}

	prevDelParams, err := ce.paramCache.Get(prevDel.ParamsVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get staking params with version %d: %w", btcDel.ParamsVersion, err)
	}

	_, prevStakingTxUnbondingPkScriptPath, err := pkScriptPathSlashAndUnbond(prevDel, prevDelParams, &ce.config.BTCNetParams)
	if err != nil {
		return nil, err
	}

	return &SigningRequestStkExp{
		PreviousActiveStakeTx:                    previousActiveStkTx,
		OtherFundingOutput:                       fundingTxOut,
		PreviousStakingOutputIdx:                 prevDel.StakingOutputIdx,
		PreviousActiveStakeUnbondingPkScriptPath: prevStakingTxUnbondingPkScriptPath,
	}, nil
}

// SignTransactions calls the signer and record metrics about signing
func (ce *Emulator) SignTransactions(signingReq SigningRequest) (*SignaturesResponse, error) {
	// record metrics
	startSignTime := time.Now()
	metricsTimeKeeper.SetPreviousSignStart(&startSignTime)

	resp, err := ce.signer.SignTransactions(signingReq)
	if err != nil {
		return nil, err
	}

	// record metrics
	finishSignTime := time.Now()
	metricsTimeKeeper.SetPreviousSignFinish(&finishSignTime)
	timedSignDelegationLag.Observe(time.Since(startSignTime).Seconds())

	return resp, nil
}

// stakeExpValidateBasic validates the stake expansion in the delegation
// It checks that the stake expansion has the correct inputs, the previous staking tx hash
// and the funding tx hash, and that the funding output has a valid value.
func stakeExpValidateBasic(btcDel *types.Delegation) (*wire.TxOut, error) {
	if btcDel == nil || btcDel.StakeExpansion == nil {
		return nil, fmt.Errorf("stake expansion is nil in the delegation")
	}

	stkExpandTx, _, err := bbntypes.NewBTCTxFromHex(btcDel.StakingTxHex)
	if err != nil {
		return nil, err
	}
	if len(stkExpandTx.TxIn) != 2 {
		return nil, fmt.Errorf("stake expansion must have 2 inputs (TxIn)")
	}

	previousActiveStkTxHash, err := chainhash.NewHashFromStr(btcDel.StakeExpansion.PreviousStakingTxHashHex)
	if err != nil {
		return nil, err
	}
	if !stkExpandTx.TxIn[0].PreviousOutPoint.Hash.IsEqual(previousActiveStkTxHash) {
		return nil, fmt.Errorf("stake expansion first input must be the previous staking transaction hash %s", btcDel.StakeExpansion.PreviousStakingTxHashHex)
	}

	otherOutput, err := btcDel.StakeExpansion.OtherFundingTxOut()
	if err != nil {
		return nil, err
	}

	// Validate funding output value
	if otherOutput.Value <= 0 {
		return nil, fmt.Errorf("funding output has invalid value %d", otherOutput.Value)
	}

	return otherOutput, nil
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
) ([]byte, error) {
	var (
		unbondingInfo *btcstaking.UnbondingInfo
		err           error
	)

	if del.IsMultisigBtcDel() {
		stakerPKs := del.MultisigInfo.StakerBtcPkList
		stakerPKs = append(stakerPKs, del.BtcPk)

		unbondingInfo, err = btcstaking.BuildMultisigUnbondingInfo(
			stakerPKs,
			del.MultisigInfo.StakerQuorum,
			del.FpBtcPks,
			params.CovenantPks,
			params.CovenantQuorum,
			del.UnbondingTime,
			btcutil.Amount(unbondingTx.TxOut[0].Value),
			btcNet,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to build multisig unbonding info: %w", err)
		}
	} else {
		unbondingInfo, err = btcstaking.BuildUnbondingInfo(
			del.BtcPk,
			del.FpBtcPks,
			params.CovenantPks,
			params.CovenantQuorum,
			del.UnbondingTime,
			btcutil.Amount(unbondingTx.TxOut[0].Value),
			btcNet,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to build unbonding info: %w", err)
		}
	}

	unbondingTxSlashingPathInfo, err := unbondingInfo.SlashingPathSpendInfo()
	if err != nil {
		return nil, err
	}
	unbondingTxSlashingScriptPath := unbondingTxSlashingPathInfo.GetPkScriptPath()

	return unbondingTxSlashingScriptPath, nil
}

func pkScriptPaths(
	del *types.Delegation,
	params *types.StakingParams,
	btcNet *chaincfg.Params,
	unbondingTx *wire.MsgTx,
) ([]byte, []byte, []byte, error) {
	slash, unbond, err := pkScriptPathSlashAndUnbond(del, params, btcNet)
	if err != nil {
		return nil, nil, nil, err
	}

	unbondSlash, err := pkScriptPathUnbondingSlash(del, unbondingTx, params, btcNet)
	if err != nil {
		return nil, nil, nil, err
	}

	return slash, unbond, unbondSlash, nil
}

func pkScriptPathSlashAndUnbond(
	del *types.Delegation,
	params *types.StakingParams,
	btcNet *chaincfg.Params,
) ([]byte, []byte, error) {
	var (
		stakingInfo *btcstaking.StakingInfo
		err         error
	)

	if del.IsMultisigBtcDel() {
		stakerPKs := del.MultisigInfo.StakerBtcPkList
		stakerPKs = append(stakerPKs, del.BtcPk)

		stakingInfo, err = btcstaking.BuildMultisigStakingInfo(
			stakerPKs,
			del.MultisigInfo.StakerQuorum,
			del.FpBtcPks,
			params.CovenantPks,
			params.CovenantQuorum,
			del.GetStakingTime(),
			del.TotalSat,
			btcNet,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build multisig staking info: %w", err)
		}
	} else {
		stakingInfo, err = btcstaking.BuildStakingInfo(
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
	}

	slashingPathInfo, err := stakingInfo.SlashingPathSpendInfo()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get slashing path info: %w", err)
	}
	slashingPkScriptPath := slashingPathInfo.GetPkScriptPath()

	// sign unbonding sig
	stakingTxUnbondingPathInfo, err := stakingInfo.UnbondingPathSpendInfo()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get unbonding path spend info")
	}
	stakingTxUnbondingPkScriptPath := stakingTxUnbondingPathInfo.GetPkScriptPath()

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

	// 2-1. verify the transactions with multisig info
	if del.IsMultisigBtcDel() {
		stakerPKs := del.MultisigInfo.StakerBtcPkList
		stakerPKs = append(stakerPKs, del.BtcPk)

		if err := btcstaking.CheckSlashingTxMatchFundingTxMultisig(
			slashingMsgTx,
			stakingMsgTx,
			del.StakingOutputIdx,
			int64(params.MinSlashingTxFeeSat),
			params.SlashingRate,
			params.SlashingPkScript,
			stakerPKs,
			del.MultisigInfo.StakerQuorum,
			del.UnbondingTime,
			btcNet,
		); err != nil {
			return nil, nil, fmt.Errorf("invalid txs in the delegation: %w", err)
		}
	}

	// 2-2. verify the transactions
	if err := btcstaking.CheckSlashingTxMatchFundingTx(
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

	// 2-1. verify transactions with multisig info
	if del.IsMultisigBtcDel() {
		stakerPKs := del.MultisigInfo.StakerBtcPkList
		stakerPKs = append(stakerPKs, del.BtcPk)

		if err := btcstaking.CheckSlashingTxMatchFundingTxMultisig(
			unbondingSlashingMsgTx,
			unbondingMsgTx,
			0,
			int64(params.MinSlashingTxFeeSat),
			params.SlashingRate,
			params.SlashingPkScript,
			stakerPKs,
			del.MultisigInfo.StakerQuorum,
			del.UnbondingTime,
			btcNet,
		); err != nil {
			return nil, nil, fmt.Errorf("invalid txs in the undelegation: %w", err)
		}
	}

	// 2-2. verify transactions
	if err := btcstaking.CheckSlashingTxMatchFundingTx(
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
func (ce *Emulator) delegationsToBatches(dels []*types.Delegation) [][]*types.Delegation {
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

// IsKeyInCommittee returns true if the covenant serialized public key is in the covenant committee of the
// parameter in which the BTC delegation was included.
func IsKeyInCommittee(paramCache ParamsGetter, covenantSerializedPk []byte, del *types.Delegation) (bool, error) {
	stkParams, err := paramCache.Get(del.ParamsVersion)
	if err != nil {
		return false, fmt.Errorf("unable to get the param version: %d, reason: %w", del.ParamsVersion, err)
	}

	for _, pk := range stkParams.CovenantPks {
		remoteKey := schnorr.SerializePubKey(pk)
		if !bytes.Equal(remoteKey, covenantSerializedPk) {
			continue
		}

		return true, nil
	}

	return false, nil
}

// CovenantAlreadySigned returns true if the covenant already signed the BTC Delegation
func AlreadySigned(covenantSerializedPk []byte, del *types.Delegation) bool {
	for _, covSig := range del.CovenantSigs {
		remoteKey := schnorr.SerializePubKey(covSig.Pk)
		if !bytes.Equal(remoteKey, covenantSerializedPk) {
			continue
		}

		return true
	}

	return false
}

// acceptDelegationToSign verifies if the delegation should be accepted to sign.
func (ce *Emulator) acceptDelegationToSign(del *types.Delegation) (bool, error) {
	var prevDel *types.Delegation
	var err error
	if del.IsStakeExpansion() {
		prevDel, err = ce.cc.QueryBTCDelegation(del.StakeExpansion.PreviousStakingTxHashHex)
		if err != nil {
			return false, fmt.Errorf("failed to query previous delegation for stake expansion: %w", err)
		}
	}

	return AcceptDelegationToSign(ce.pk, ce.paramCache, del, prevDel)
}

// AcceptDelegationToSign returns true if the delegation should be accepted to be signed.
// Returns false if the covenant public key already signed
// or if the delegation was not constructed with that covenant public key.
func AcceptDelegationToSign(
	pk *btcec.PublicKey,
	paramCache ParamsGetter,
	del *types.Delegation,
	prevDel *types.Delegation, // for stake expansion, previous delegation
) (bool, error) {
	covenantSerializedPk := schnorr.SerializePubKey(pk)
	// 1. Check if the delegation does not need the covenant's signature because
	// this covenant already signed
	if AlreadySigned(covenantSerializedPk, del) {
		return false, nil
	}

	// 2. Check if the delegation was not constructed with this covenant public key
	isInCommittee, err := IsKeyInCommittee(paramCache, covenantSerializedPk, del)
	if err != nil {
		return false, fmt.Errorf("unable to verify if covenant key is in committee: %w", err)
	}
	if !isInCommittee {
		return false, nil
	}
	// 3. For stake expansion, verify if the covenant is in the committee of the previous active delegation
	if del.IsStakeExpansion() {
		valid, err := ValidateStakeExpansion(paramCache, covenantSerializedPk, del, prevDel)
		if err != nil {
			return false, err
		}
		if !valid {
			return false, fmt.Errorf("covenant %s is not in the committee of the previous delegation %s",
				hex.EncodeToString(covenantSerializedPk), del.StakeExpansion.PreviousStakingTxHashHex)
		}
	}

	return true, nil
}

// ValidateStakeExpansion validates that a stake expansion delegation is properly configured
// and that the covenant was in the committee for the previous delegation.
func ValidateStakeExpansion(
	paramCache ParamsGetter,
	covenantSerializedPk []byte,
	del *types.Delegation,
	prevDel *types.Delegation,
) (bool, error) {
	if prevDel == nil {
		return false, fmt.Errorf("previous delegation is nil for stake expansion delegation: %s", del.StakeExpansion.PreviousStakingTxHashHex)
	}

	// Validate that the previous delegation's staking transaction hash matches the expected one
	prevStakingTx, _, err := bbntypes.NewBTCTxFromHex(prevDel.StakingTxHex)
	if err != nil {
		return false, fmt.Errorf("failed to decode previous delegation staking tx: %w", err)
	}
	prevStakingTxHash := prevStakingTx.TxHash().String()

	if prevStakingTxHash != del.StakeExpansion.PreviousStakingTxHashHex {
		return false, fmt.Errorf("previous delegation staking tx hash mismatch: expected %s, got %s",
			del.StakeExpansion.PreviousStakingTxHashHex, prevStakingTxHash)
	}

	// Verify that the current covenant was in the committee for the previous delegation
	isInCommittee, err := IsKeyInCommittee(paramCache, covenantSerializedPk, prevDel)
	if err != nil {
		return false, fmt.Errorf("unable to verify if covenant key is in committee: %w", err)
	}
	if !isInCommittee {
		return false, nil
	}

	return true, nil
}

// covenantSigSubmissionLoop is the reactor to submit Covenant signature for BTC delegations
func (ce *Emulator) covenantSigSubmissionLoop() {
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
			dels, err := ce.cc.QueryPendingDelegations(limit, ce.acceptDelegationToSign)
			if err != nil {
				ce.logger.Debug("failed to get pending delegations", zap.Error(err))

				continue
			}

			pendingDels := len(dels)
			// record delegation metrics
			ce.recordMetricsCurrentPendingDelegations(pendingDels)

			if pendingDels == 0 {
				ce.logger.Debug("no pending delegations are found")

				continue
			}

			// 2. Split delegations into batches for submission
			batches := ce.delegationsToBatches(dels)
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

func (ce *Emulator) metricsUpdateLoop() {
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

func (ce *Emulator) recordMetricsFailedSignDelegations(n int) {
	failedSignDelegations.WithLabelValues(ce.PublicKeyStr()).Add(float64(n))
}

func (ce *Emulator) recordMetricsTotalSignDelegationsSubmitted(n int) {
	totalSignDelegationsSubmitted.WithLabelValues(ce.PublicKeyStr()).Add(float64(n))
}

func (ce *Emulator) recordMetricsCurrentPendingDelegations(n int) {
	currentPendingDelegations.WithLabelValues(ce.PublicKeyStr()).Set(float64(n))
}

func (ce *Emulator) Start() error {
	var startErr error
	ce.startOnce.Do(func() {
		ce.logger.Info("Starting Covenant Emulator")

		ce.wg.Add(2)
		go ce.covenantSigSubmissionLoop()
		go ce.metricsUpdateLoop()
	})

	return startErr
}

func (ce *Emulator) Stop() error {
	var stopErr error
	ce.stopOnce.Do(func() {
		ce.logger.Info("Stopping Covenant Emulator")

		close(ce.quit)
		ce.wg.Wait()

		ce.logger.Debug("Covenant Emulator successfully stopped")
	})

	return stopErr
}

// CheckReadiness checks if the covenant emulator is ready to serve requests.
// It verifies internal state (if the main loop is running) and connectivity to
// dependencies (remote signer).
func (ce *Emulator) CheckReadiness() error {
	select {
	case <-ce.quit:
		return fmt.Errorf("emulator is not running")
	default:
		// Emulator is running
	}

	// Check connectivity to the remote signer by calling its PubKey endpoint
	_, err := ce.signer.PubKey()
	if err != nil {
		return fmt.Errorf("failed to get public key from covenant signer: %w", err)
	}

	return nil
}
