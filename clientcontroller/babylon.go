package clientcontroller

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	sdkErrors "cosmossdk.io/errors"

	sdkmath "cosmossdk.io/math"
	"github.com/btcsuite/btcd/btcec/v2"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/v3/x/btclightclient/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/covenant-emulator/config"
	"github.com/babylonlabs-io/covenant-emulator/types"
)

var (
	_                  ClientController = &BabylonController{}
	MaxPaginationLimit                  = uint64(1000)
	messageIndexRegex                   = regexp.MustCompile(`message index:\s*(\d+)`)
)

type BabylonController struct {
	bbnClient *bbnclient.Client
	cfg       *config.BBNConfig
	btcParams *chaincfg.Params
	logger    *zap.Logger

	MaxRetiresBatchRemovingMsgs uint64
}

func NewBabylonController(
	cfg *config.BBNConfig,
	btcParams *chaincfg.Params,
	logger *zap.Logger,
	maxRetiresBatchRemovingMsgs uint64,
) (*BabylonController, error) {
	bbnConfig := config.BBNConfigToBabylonConfig(cfg)

	if err := bbnConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config for Babylon client: %w", err)
	}

	bc, err := bbnclient.New(
		&bbnConfig,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Babylon client: %w", err)
	}

	return &BabylonController{
		bc,
		cfg,
		btcParams,
		logger,
		maxRetiresBatchRemovingMsgs,
	}, nil
}

func (bc *BabylonController) mustGetTxSigner() string {
	signer := bc.GetKeyAddress()
	prefix := bc.cfg.AccountPrefix

	return sdk.MustBech32ifyAddressBytes(prefix, signer)
}

func (bc *BabylonController) GetKeyAddress() sdk.AccAddress {
	// get key address, retrieves address based on key name which is configured in
	// cfg *stakercfg.BBNConfig. If this fails, it means we have misconfiguration problem
	// and we should panic.
	// This is checked at the start of BabylonController, so if it fails something is really wrong

	keyRec, err := bc.bbnClient.GetKeyring().Key(bc.cfg.Key)

	if err != nil {
		panic(fmt.Sprintf("Failed to get key address: %s", err))
	}

	addr, err := keyRec.GetAddress()

	if err != nil {
		panic(fmt.Sprintf("Failed to get key address: %s", err))
	}

	return addr
}

func (bc *BabylonController) QueryStakingParamsByVersion(version uint32) (*types.StakingParams, error) {
	// query btc checkpoint params
	ckptParamRes, err := bc.bbnClient.BTCCheckpointParams()
	if err != nil {
		return nil, fmt.Errorf("failed to query params of the btccheckpoint module: %w", err)
	}

	// query btc staking params
	stakingParamRes, err := bc.bbnClient.BTCStakingParamsByVersion(version)
	if err != nil {
		return nil, fmt.Errorf("failed to query staking params with version %d: %w", version, err)
	}

	covenantPks := make([]*btcec.PublicKey, 0, len(stakingParamRes.Params.CovenantPks))
	for _, pk := range stakingParamRes.Params.CovenantPks {
		covPk, err := pk.ToBTCPK()
		if err != nil {
			return nil, fmt.Errorf("invalid covenant public key")
		}
		covenantPks = append(covenantPks, covPk)
	}

	if stakingParamRes.Params.MinStakingTimeBlocks > math.MaxUint16 {
		return nil, fmt.Errorf("babylon min staking time blocks (%d) is larger than the maximum uint16", stakingParamRes.Params.MinStakingTimeBlocks)
	}
	// #nosec G115 -- performed the conversion check above
	minStakingTimeBlocksUint16 := uint16(stakingParamRes.Params.MinStakingTimeBlocks)

	if stakingParamRes.Params.MaxStakingTimeBlocks > math.MaxUint16 {
		return nil, fmt.Errorf("babylon max staking time blocks (%d) is larger than the maximum uint16", stakingParamRes.Params.MaxStakingTimeBlocks)
	}
	// #nosec G115 -- performed the conversion check above
	maxStakingTimeBlocksUint16 := uint16(stakingParamRes.Params.MaxStakingTimeBlocks)

	return &types.StakingParams{
		ComfirmationTimeBlocks:    ckptParamRes.Params.BtcConfirmationDepth,
		FinalizationTimeoutBlocks: ckptParamRes.Params.CheckpointFinalizationTimeout,
		MinSlashingTxFeeSat:       btcutil.Amount(stakingParamRes.Params.MinSlashingTxFeeSat),
		CovenantPks:               covenantPks,
		SlashingPkScript:          stakingParamRes.Params.SlashingPkScript,
		CovenantQuorum:            stakingParamRes.Params.CovenantQuorum,
		SlashingRate:              stakingParamRes.Params.SlashingRate,
		MinComissionRate:          stakingParamRes.Params.MinCommissionRate,
		UnbondingTimeBlocks:       stakingParamRes.Params.UnbondingTimeBlocks,
		UnbondingFee:              btcutil.Amount(stakingParamRes.Params.UnbondingFeeSat),
		MinStakingTime:            minStakingTimeBlocksUint16,
		MaxStakingTime:            maxStakingTimeBlocksUint16,
		MinStakingValue:           btcutil.Amount(stakingParamRes.Params.MinStakingValueSat),
		MaxStakingValue:           btcutil.Amount(stakingParamRes.Params.MaxStakingValueSat),
	}, nil
}

func (bc *BabylonController) reliablySendMsg(msg sdk.Msg) (*babylonclient.RelayerTxResponse, error) {
	return bc.reliablySendMsgs([]sdk.Msg{msg})
}

func (bc *BabylonController) reliablySendMsgs(msgs []sdk.Msg) (*babylonclient.RelayerTxResponse, error) {
	return bc.bbnClient.ReliablySendMsgs(
		context.Background(),
		msgs,
		nil,
		unrecoverableErrors,
	)
}

// SubmitCovenantSigs submits the Covenant signature via a MsgAddCovenantSig to Babylon if the daemon runs in Covenant mode
// it returns tx hash and error
func (bc *BabylonController) SubmitCovenantSigs(covSigs []*types.CovenantSigs) (*types.TxResponse, error) {
	msgs := make([]sdk.Msg, 0, len(covSigs))
	for _, covSig := range covSigs {
		bip340UnbondingSig := bbntypes.NewBIP340SignatureFromBTCSig(covSig.UnbondingSig)
		msg := &btcstakingtypes.MsgAddCovenantSigs{
			Signer:                  bc.mustGetTxSigner(),
			Pk:                      bbntypes.NewBIP340PubKeyFromBTCPK(covSig.PublicKey),
			StakingTxHash:           covSig.StakingTxHash.String(),
			SlashingTxSigs:          covSig.SlashingSigs,
			UnbondingTxSig:          bip340UnbondingSig,
			SlashingUnbondingTxSigs: covSig.SlashingUnbondingSigs,
			StakeExpansionTxSig:     nil,
		}

		if covSig.StkExpSig != nil {
			stkExpSig := bbntypes.NewBIP340SignatureFromBTCSig(covSig.StkExpSig)
			msg.StakeExpansionTxSig = stkExpSig
		}

		msgs = append(msgs, msg)
	}

	return bc.reliablySendMsgsResendingOnMsgErr(msgs)
}

// reliablySendMsgsResendingOnMsgErr sends the msgs to the chain, if some msg fails to execute
// and contains 'message index: %d', it will remove that msg from the batch and send again
// if there is no more message available, returns the last error.
func (bc *BabylonController) reliablySendMsgsResendingOnMsgErr(msgs []sdk.Msg) (*types.TxResponse, error) {
	var err error

	maxRetries := BatchRetries(msgs, bc.MaxRetiresBatchRemovingMsgs)
	for i := uint64(0); i < maxRetries; i++ {
		res, errSendMsg := bc.reliablySendMsgs(msgs)
		if errSendMsg != nil {
			// concatenate the errors, to throw out if needed
			err = errors.Join(err, errSendMsg)

			// something failed, check if it is the message index failure
			//if len(msgs) <= 1 {
			//	return nil, err
			//}

			if strings.Contains(errSendMsg.Error(), "message index: ") {
				// remove the failed msg from the batch and send again
				failedIndex, found := FailedMessageIndex(errSendMsg)
				if !found {
					return nil, errSendMsg
				}

				msgs = RemoveMsgAtIndex(msgs, failedIndex)

				continue
			}

			return nil, errSendMsg
		}

		if res == nil { // expected error happened
			return &types.TxResponse{}, nil
		}

		return &types.TxResponse{TxHash: res.TxHash, Events: res.Events}, nil
	}

	if err != nil && errorContained(err, expectedErrors) {
		return &types.TxResponse{}, nil
	}

	return nil, fmt.Errorf("failed to send batch of msgs: %w", err)
}

// BatchRetries returns the max number of retries it should execute based on the
// amount of messages in the batch
func BatchRetries(msgs []sdk.Msg, maxRetiresBatchRemovingMsgs uint64) uint64 {
	maxRetriesByMsgLen := uint64(len(msgs))

	if maxRetiresBatchRemovingMsgs == 0 {
		return maxRetriesByMsgLen
	}

	if maxRetiresBatchRemovingMsgs > maxRetriesByMsgLen {
		return maxRetriesByMsgLen
	}

	return maxRetiresBatchRemovingMsgs
}

// RemoveMsgAtIndex removes any msg inside the slice, based on the index is given
// if the index is out of bounds, it just returns the slice of msgs.
func RemoveMsgAtIndex(msgs []sdk.Msg, index int) []sdk.Msg {
	if index < 0 || index >= len(msgs) {
		return msgs
	}

	return append(msgs[:index], msgs[index+1:]...)
}

// FailedMessageIndex finds the message index which failed in a error which contains
// the substring 'message index: %d'.
// ex.:  rpc error: code = Unknown desc = failed to execute message; message index: 1: the covenant signature is already submitted
func FailedMessageIndex(err error) (int, bool) {
	matches := messageIndexRegex.FindStringSubmatch(err.Error())

	if len(matches) > 1 {
		index, errAtoi := strconv.Atoi(matches[1])
		if errAtoi == nil {
			return index, true
		}
	}

	return 0, false
}

func (bc *BabylonController) QueryPendingDelegations(limit uint64, filter FilterFn) ([]*types.Delegation, error) {
	return bc.queryDelegationsWithStatus(btcstakingtypes.BTCDelegationStatus_PENDING, limit, filter)
}

func (bc *BabylonController) QueryActiveDelegations(limit uint64) ([]*types.Delegation, error) {
	return bc.queryDelegationsWithStatus(btcstakingtypes.BTCDelegationStatus_ACTIVE, limit, nil)
}

func (bc *BabylonController) QueryVerifiedDelegations(limit uint64) ([]*types.Delegation, error) {
	return bc.queryDelegationsWithStatus(btcstakingtypes.BTCDelegationStatus_VERIFIED, limit, nil)
}

// QueryBTCDelegation queries the BTC delegation by the tx hash
func (bc *BabylonController) QueryBTCDelegation(stakingTxHashHex string) (*types.Delegation, error) {
	resp, err := bc.bbnClient.BTCDelegation(stakingTxHashHex)
	if err != nil {
		return nil, fmt.Errorf("failed to query BTC delegation %s: %w", stakingTxHashHex, err)
	}

	return DelegationRespToDelegation(resp.BtcDelegation)
}

// queryDelegationsWithStatus queries BTC delegations that need a Covenant signature
// with the given status (either pending or unbonding)
// it is only used when the program is running in Covenant mode
func (bc *BabylonController) queryDelegationsWithStatus(status btcstakingtypes.BTCDelegationStatus, delsLimit uint64, filter FilterFn) ([]*types.Delegation, error) {
	pgLimit := min(MaxPaginationLimit, delsLimit)
	pagination := &sdkquery.PageRequest{
		Limit: pgLimit,
	}

	dels := make([]*types.Delegation, 0, delsLimit)
	indexDels := uint64(0)

	for indexDels < delsLimit {
		res, err := bc.bbnClient.BTCDelegations(status, pagination)
		if err != nil {
			return nil, fmt.Errorf("failed to query BTC delegations: %w", err)
		}

		for _, delResp := range res.BtcDelegations {
			del, err := DelegationRespToDelegation(delResp)
			if err != nil {
				return nil, err
			}

			if filter != nil {
				accept, err := filter(del)
				if err != nil {
					return nil, err
				}

				if !accept {
					continue
				}
			}

			dels = append(dels, del)
			indexDels++

			if indexDels == delsLimit {
				return dels, nil
			}
		}

		// if returned a different number of btc delegations than the pagination limit
		// it means that there is no more delegations at the store
		if uint64(len(res.BtcDelegations)) != pgLimit {
			return dels, nil
		}
		pagination.Key = res.Pagination.NextKey
	}

	return dels, nil
}

func getContextWithCancel(timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	return ctx, cancel
}

func (bc *BabylonController) Close() error {
	if !bc.bbnClient.IsRunning() {
		return nil
	}

	return bc.bbnClient.Stop()
}

func DelegationRespToDelegation(del *btcstakingtypes.BTCDelegationResponse) (*types.Delegation, error) {
	var (
		covenantSigs []*types.CovenantAdaptorSigInfo
		undelegation *types.Undelegation
		err          error
	)

	if del.StakingTxHex == "" {
		return nil, fmt.Errorf("staking tx should not be empty in delegation")
	}

	if del.SlashingTxHex == "" {
		return nil, fmt.Errorf("slashing tx should not be empty in delegation")
	}

	for _, s := range del.CovenantSigs {
		covSigInfo := &types.CovenantAdaptorSigInfo{
			Pk:   s.CovPk.MustToBTCPK(),
			Sigs: s.AdaptorSigs,
		}
		covenantSigs = append(covenantSigs, covSigInfo)
	}

	if del.UndelegationResponse != nil {
		undelegation, err = UndelegationRespToUndelegation(del.UndelegationResponse)
		if err != nil {
			return nil, err
		}
	}

	fpBtcPks := make([]*btcec.PublicKey, 0, len(del.FpBtcPkList))
	for _, fp := range del.FpBtcPkList {
		fpBtcPks = append(fpBtcPks, fp.MustToBTCPK())
	}

	if del.UnbondingTime > uint32(math.MaxUint16) {
		return nil, fmt.Errorf("unbonding time should be smaller than max uint16")
	}

	if del.TotalSat > uint64(math.MaxInt64) {
		return nil, fmt.Errorf("total sat (%d) is larger than the maximum int64", del.TotalSat)
	}

	respDel := &types.Delegation{
		BtcPk:            del.BtcPk.MustToBTCPK(),
		FpBtcPks:         fpBtcPks,
		TotalSat:         btcutil.Amount(del.TotalSat),
		StakingTime:      del.StakingTime,
		StartHeight:      del.StartHeight,
		EndHeight:        del.EndHeight,
		StakingTxHex:     del.StakingTxHex,
		SlashingTxHex:    del.SlashingTxHex,
		StakingOutputIdx: del.StakingOutputIdx,
		CovenantSigs:     covenantSigs,
		UnbondingTime:    uint16(del.UnbondingTime),
		BtcUndelegation:  undelegation,
		ParamsVersion:    del.ParamsVersion,
		StakeExpansion:   nil,
	}

	if del.StkExp != nil {
		respDel.StakeExpansion = &types.DelegationStakeExpansion{
			PreviousStakingTxHashHex: del.StkExp.PreviousStakingTxHashHex,
			OtherFundingTxOutHex:     del.StkExp.OtherFundingTxOutHex,
		}
	}

	return respDel, nil
}

func UndelegationRespToUndelegation(undel *btcstakingtypes.BTCUndelegationResponse) (*types.Undelegation, error) {
	var (
		covenantSlashingSigs  []*types.CovenantAdaptorSigInfo
		covenantUnbondingSigs []*types.CovenantSchnorrSigInfo
	)

	if undel.UnbondingTxHex == "" {
		return nil, fmt.Errorf("staking tx should not be empty in undelegation")
	}

	if undel.SlashingTxHex == "" {
		return nil, fmt.Errorf("slashing tx should not be empty in undelegation")
	}

	for _, unbondingSig := range undel.CovenantUnbondingSigList {
		sig, err := unbondingSig.Sig.ToBTCSig()
		if err != nil {
			return nil, err
		}
		sigInfo := &types.CovenantSchnorrSigInfo{
			Pk:  unbondingSig.Pk.MustToBTCPK(),
			Sig: sig,
		}
		covenantUnbondingSigs = append(covenantUnbondingSigs, sigInfo)
	}

	for _, s := range undel.CovenantSlashingSigs {
		covSigInfo := &types.CovenantAdaptorSigInfo{
			Pk:   s.CovPk.MustToBTCPK(),
			Sigs: s.AdaptorSigs,
		}
		covenantSlashingSigs = append(covenantSlashingSigs, covSigInfo)
	}

	var spendStakeTxHex = ""
	if undel.DelegatorUnbondingInfoResponse != nil {
		spendStakeTxHex = undel.DelegatorUnbondingInfoResponse.SpendStakeTxHex
	}

	return &types.Undelegation{
		UnbondingTxHex:        undel.UnbondingTxHex,
		SlashingTxHex:         undel.SlashingTxHex,
		CovenantSlashingSigs:  covenantSlashingSigs,
		CovenantUnbondingSigs: covenantUnbondingSigs,
		SpendStakeTxHex:       spendStakeTxHex,
	}, nil
}

// Currently this is only used for e2e tests, probably does not need to add it into the interface
func (bc *BabylonController) CreateBTCDelegation(
	delBtcPk *bbntypes.BIP340PubKey,
	fpPks []*btcec.PublicKey,
	pop *btcstakingtypes.ProofOfPossessionBTC,
	stakingTime uint32,
	stakingValue int64,
	stakingTxInfo *btcctypes.TransactionInfo,
	slashingTx *btcstakingtypes.BTCSlashingTx,
	delSlashingSig *bbntypes.BIP340Signature,
	unbondingTx []byte,
	unbondingTime uint32,
	unbondingValue int64,
	unbondingSlashingTx *btcstakingtypes.BTCSlashingTx,
	delUnbondingSlashingSig *bbntypes.BIP340Signature,
	isPreApproval bool,
) (*types.TxResponse, error) {
	fpBtcPks := make([]bbntypes.BIP340PubKey, 0, len(fpPks))
	for _, v := range fpPks {
		fpBtcPks = append(fpBtcPks, *bbntypes.NewBIP340PubKeyFromBTCPK(v))
	}

	var inclusionProof *btcstakingtypes.InclusionProof
	if !isPreApproval {
		inclusionProof = btcstakingtypes.NewInclusionProof(stakingTxInfo.Key, stakingTxInfo.Proof)
	}

	msg := &btcstakingtypes.MsgCreateBTCDelegation{
		StakerAddr:                    bc.mustGetTxSigner(),
		Pop:                           pop,
		BtcPk:                         delBtcPk,
		FpBtcPkList:                   fpBtcPks,
		StakingTime:                   stakingTime,
		StakingValue:                  stakingValue,
		StakingTx:                     stakingTxInfo.Transaction,
		StakingTxInclusionProof:       inclusionProof,
		SlashingTx:                    slashingTx,
		DelegatorSlashingSig:          delSlashingSig,
		UnbondingTx:                   unbondingTx,
		UnbondingTime:                 unbondingTime,
		UnbondingValue:                unbondingValue,
		UnbondingSlashingTx:           unbondingSlashingTx,
		DelegatorUnbondingSlashingSig: delUnbondingSlashingSig,
	}

	res, err := bc.reliablySendMsg(msg)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// CreateStakeExpansionDelegation creates a BTC stake expansion delegation using MsgBtcStakeExpand
// Currently this is only used for e2e tests, probably does not need to add it into the interface
func (bc *BabylonController) CreateStakeExpansionDelegation(
	delBtcPk *bbntypes.BIP340PubKey,
	fpPks []*btcec.PublicKey,
	pop *btcstakingtypes.ProofOfPossessionBTC,
	stakingTime uint32,
	stakingValue int64,
	stakingTxInfo *btcctypes.TransactionInfo,
	slashingTx *btcstakingtypes.BTCSlashingTx,
	delSlashingSig *bbntypes.BIP340Signature,
	unbondingTx []byte,
	unbondingTime uint32,
	unbondingValue int64,
	unbondingSlashingTx *btcstakingtypes.BTCSlashingTx,
	delUnbondingSlashingSig *bbntypes.BIP340Signature,
	previousStakingTxHash string,
	fundingTx []byte,
) (*types.TxResponse, error) {
	fpBtcPks := make([]bbntypes.BIP340PubKey, 0, len(fpPks))
	for _, v := range fpPks {
		fpBtcPks = append(fpBtcPks, *bbntypes.NewBIP340PubKeyFromBTCPK(v))
	}

	msg := &btcstakingtypes.MsgBtcStakeExpand{
		StakerAddr:                    bc.mustGetTxSigner(),
		Pop:                           pop,
		BtcPk:                         delBtcPk,
		FpBtcPkList:                   fpBtcPks,
		StakingTime:                   stakingTime,
		StakingValue:                  stakingValue,
		StakingTx:                     stakingTxInfo.Transaction,
		SlashingTx:                    slashingTx,
		DelegatorSlashingSig:          delSlashingSig,
		UnbondingTx:                   unbondingTx,
		UnbondingTime:                 unbondingTime,
		UnbondingValue:                unbondingValue,
		UnbondingSlashingTx:           unbondingSlashingTx,
		DelegatorUnbondingSlashingSig: delUnbondingSlashingSig,
		PreviousStakingTxHash:         previousStakingTxHash,
		FundingTx:                     fundingTx,
	}

	res, err := bc.reliablySendMsg(msg)
	if err != nil {
		return nil, err
	}

	return &types.TxResponse{TxHash: res.TxHash}, nil
}

// Register a finality provider to Babylon
// Currently this is only used for e2e tests, probably does not need to add it into the interface
func (bc *BabylonController) RegisterFinalityProvider(
	btcPubKey *bbntypes.BIP340PubKey, commission *sdkmath.LegacyDec,
	description *stakingtypes.Description, pop *btcstakingtypes.ProofOfPossessionBTC) (*babylonclient.RelayerTxResponse, error) {
	registerMsg := &btcstakingtypes.MsgCreateFinalityProvider{
		Addr:        bc.mustGetTxSigner(),
		Commission:  btcstakingtypes.NewCommissionRates(*commission, commission.Add(sdkmath.LegacyOneDec()), sdkmath.LegacyOneDec()),
		BtcPk:       btcPubKey,
		Description: description,
		Pop:         pop,
	}

	return bc.reliablySendMsgs([]sdk.Msg{registerMsg})
}

// Insert BTC block header using rpc client
// Currently this is only used for e2e tests, probably does not need to add it into the interface
func (bc *BabylonController) InsertBtcBlockHeaders(headers []bbntypes.BTCHeaderBytes) (*babylonclient.RelayerTxResponse, error) {
	msg := &btclctypes.MsgInsertHeaders{
		Signer:  bc.mustGetTxSigner(),
		Headers: headers,
	}

	res, err := bc.reliablySendMsg(msg)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// QueryFinalityProvider queries finality providers
// Currently this is only used for e2e tests, probably does not need to add this into the interface
func (bc *BabylonController) QueryFinalityProviders() ([]*btcstakingtypes.FinalityProviderResponse, error) {
	var fps []*btcstakingtypes.FinalityProviderResponse
	pagination := &sdkquery.PageRequest{
		Limit: 100,
	}

	ctx, cancel := getContextWithCancel(bc.cfg.Timeout)
	defer cancel()

	clientCtx := sdkclient.Context{Client: bc.bbnClient.RPCClient}

	queryClient := btcstakingtypes.NewQueryClient(clientCtx)

	for {
		queryRequest := &btcstakingtypes.QueryFinalityProvidersRequest{
			Pagination: pagination,
		}
		res, err := queryClient.FinalityProviders(ctx, queryRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to query finality providers: %w", err)
		}
		fps = append(fps, res.FinalityProviders...)
		if res.Pagination == nil || res.Pagination.NextKey == nil {
			break
		}

		pagination.Key = res.Pagination.NextKey
	}

	return fps, nil
}

// Currently this is only used for e2e tests, probably does not need to add this into the interface
func (bc *BabylonController) QueryBtcLightClientTip() (*btclctypes.BTCHeaderInfoResponse, error) {
	ctx, cancel := getContextWithCancel(bc.cfg.Timeout)
	defer cancel()

	clientCtx := sdkclient.Context{Client: bc.bbnClient.RPCClient}

	queryClient := btclctypes.NewQueryClient(clientCtx)

	queryRequest := &btclctypes.QueryTipRequest{}
	res, err := queryClient.Tip(ctx, queryRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to query BTC tip: %w", err)
	}

	return res.Header, nil
}

func errorContained(err error, errList []*sdkErrors.Error) bool {
	for _, e := range errList {
		if strings.Contains(err.Error(), e.Error()) {
			return true
		}
	}

	return false
}
