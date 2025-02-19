package clientcontroller

import (
	"context"
	"fmt"
	"math"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"errors"

	sdkerrors "cosmossdk.io/errors"
	"cosmossdk.io/store/rootmulti"
	"github.com/avast/retry-go/v4"
	appparams "github.com/babylonlabs-io/babylon/app/params"
	"github.com/babylonlabs-io/babylon/client/babylonclient"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	"github.com/babylonlabs-io/babylon/client/config"
	abci "github.com/cometbft/cometbft/abci/types"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	legacyerrors "github.com/cosmos/cosmos-sdk/types/errors"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/juju/fslock"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Note: most of the functions were adappt from
// https://github.com/babylonlabs-io/babylon/blob/cd0bbcd98be5e4dda081f7330140cf9dbee4c94d/client/client/tx.go#L76
var (
	rtyAttNum                   = uint(5)
	rtyAtt                      = retry.Attempts(rtyAttNum)
	rtyDel                      = retry.Delay(time.Millisecond * 400)
	rtyErr                      = retry.LastErrorOnly(true)
	defaultBroadcastWaitTimeout = 10 * time.Minute
	srcChanTag                  = "packet_src_channel"
	dstChanTag                  = "packet_dst_channel"
)

// callbackTx is the expected type that waits for the inclusion of a transaction on the chain to be called
type callbackTx func(*sdk.TxResponse, error)

type failedMsg struct {
	msg    sdk.Msg
	reason error
}

// reliablySendEachMsgAsTx creates multiple
func reliablySendEachMsgAsTx(
	cfg *config.BabylonConfig,
	cp *babylonclient.CosmosProvider,
	msgs []sdk.Msg,
	log *zap.Logger,
	encCfg *appparams.EncodingConfig,
	covAcc sdk.AccountI,
) (txResponses []*sdk.TxResponse, failedMsgs []*failedMsg, err error) {
	c, err := http.NewWithTimeout(cfg.RPCAddr, "/websocket", uint(cfg.Timeout.Seconds()))
	if err != nil {
		return nil, nil, err
	}
	rpcClient := babylonclient.NewRPCClient(c)

	ctx := context.Background()

	msgLen := len(msgs)
	// create outputs at msg len capacity to handle each msg in parallel
	// as it is easier than pass 2 channels for each func
	txResponses = make([]*sdk.TxResponse, msgLen)
	failedMsgs = make([]*failedMsg, msgLen)

	var wg sync.WaitGroup

	accSequence := covAcc.GetSequence()
	accNumber := covAcc.GetAccountNumber()

	for msgIndex, msg := range msgs {
		wg.Add(1)

		callback := reliablySendEachMsgAsTxCallback(log, &wg, msg, msgIndex, txResponses, failedMsgs)

		go func(
			ctx context.Context,
			cfg *config.BabylonConfig,
			cp *babylonclient.CosmosProvider,
			log *zap.Logger,
			rpcClient *babylonclient.RPCClient,
			encCfg *appparams.EncodingConfig,
			msg sdk.Msg,
			accSequence, accNumber uint64,
			callback callbackTx,

			msgIndex int,
		) {
			err := RetrySendMessagesToMempool(ctx, cfg, cp, log, rpcClient, encCfg, []sdk.Msg{msg}, accSequence, accNumber, callback)
			if err != nil {
				if ErrorContained(err, expectedErrors) {
					log.Error("expected err when submitting the tx, skip retrying", zap.Error(err))
				} else {
					log.Error("failed to retry message", zap.Int("msg_index", msgIndex), zap.Error(err))
				}
				// If the callback was not invoked, decrement the wait group here
				wg.Done()
			}
		}(ctx, cfg, cp, log, &rpcClient, encCfg, msg, accSequence, accNumber, callback, msgIndex)

		accSequence++
	}

	wg.Wait()

	return CleanSlice(txResponses), CleanSlice(failedMsgs), nil
}

func RetrySendMessagesToMempool(
	ctx context.Context,
	cfg *config.BabylonConfig,
	cp *babylonclient.CosmosProvider,
	log *zap.Logger,
	rpcClient *babylonclient.RPCClient,
	encCfg *appparams.EncodingConfig,

	msgs []sdk.Msg,

	accSequence, accNumber uint64,

	asyncCallbacks ...callbackTx,
) error {
	return retry.Do(func() error {
		sendMsgErr := SendMessagesToMempool(ctx, cfg, cp, log, rpcClient, encCfg, msgs, accSequence, accNumber, asyncCallbacks...)
		if sendMsgErr != nil {
			if ErrorContained(sendMsgErr, unrecoverableErrors) {
				log.Error("unrecoverable err when submitting the tx, skip retrying", zap.Error(sendMsgErr))
				return retry.Unrecoverable(sendMsgErr)
			}
			return sendMsgErr
		}
		return nil
	}, retry.Context(ctx), rtyAtt, rtyDel, rtyErr, retry.OnRetry(func(n uint, err error) {
		log.Debug("retrying", zap.Uint("attempt", n+1), zap.Uint("max_attempts", rtyAttNum), zap.Error(err))
	}))
}

// SendMessagesToMempool simulates and broadcasts a transaction with the given msgs and memo.
// This method will return once the transaction has entered the mempool.
// In an async goroutine, will wait for the tx to be included in the block unless asyncCtx exits.
// If there is no error broadcasting, the asyncCallback will be called with success/failure of the wait for block inclusion.
func SendMessagesToMempool(
	ctx context.Context,
	cfg *config.BabylonConfig,
	cp *babylonclient.CosmosProvider,
	logger *zap.Logger,
	rpcClient *babylonclient.RPCClient,
	encCfg *appparams.EncodingConfig,

	msgs []sdk.Msg,

	accSequence, accNumber uint64,

	asyncCallbacks ...callbackTx,
) error {
	memo, gas := "", uint64(0)

	txBytes, _, err := BuildMessages(ctx, cfg, cp, encCfg, msgs, accSequence, accNumber, memo, gas)
	if err != nil {
		return err
	}

	err = BroadcastTx(ctx, logger, cfg, encCfg, rpcClient, txBytes, msgs, ctx, defaultBroadcastWaitTimeout, asyncCallbacks)
	if err != nil {
		return err
	}

	return nil
}

func BuildMessages(
	ctx context.Context,
	cfg *config.BabylonConfig,
	cp *babylonclient.CosmosProvider,
	encCfg *appparams.EncodingConfig,
	msgs []sdk.Msg,
	accSequence, accNumber uint64,
	// optional
	memo string,
	gas uint64,
) (
	txBytes []byte,
	fees sdk.Coins,
	err error,
) {
	keybase, err := KeybaseFromCfg(cfg, encCfg.Codec)
	if err != nil {
		return nil, sdk.Coins{}, err
	}

	txf := TxFactory(cfg, encCfg.TxConfig, keybase)
	if memo != "" {
		txf = txf.WithMemo(memo)
	}

	txf = txf.WithSequence(accSequence).
		WithAccountNumber(accNumber)

	txSignerKey := cfg.Key

	ws := &babylonclient.WalletState{
		NextAccountSequence: accSequence,
		Mu:                  sync.Mutex{},
	}

	relayerMsgs := bbnclient.ToProviderMsgs(msgs)
	txBytes, _, fees, err = cp.BuildMessages(ctx, txf, relayerMsgs, memo, 0, txSignerKey, ws)
	if err != nil {
		return nil, sdk.Coins{}, err
	}

	// adjusted := gas
	// if gas == 0 {
	// 	_, adjusted, err = cp.CalculateGas(ctx, txf, txSignerKey, msgs...)
	// 	if err != nil {
	// 		return nil, sdk.Coins{}, err
	// 	}
	// }

	// // Set the gas amount on the transaction factory
	// txf = txf.WithGas(adjusted)

	// // Build the transaction builder
	// txb, err := txf.BuildUnsignedTx(msgs...)
	// if err != nil {
	// 	return nil, sdk.Coins{}, err
	// }

	// if err = tx.Sign(ctx, txf, txSignerKey, txb, false); err != nil {
	// 	return nil, sdk.Coins{}, err
	// }

	// tx := txb.GetTx()
	// fees = tx.GetFee()

	// // Generate the transaction bytes
	// txBytes, err = encCfg.TxConfig.TxEncoder()(tx)
	// if err != nil {
	// 	return nil, sdk.Coins{}, err
	// }

	return txBytes, fees, nil
}

// BroadcastTx broadcasts a transaction with the given raw bytes and then, in an async goroutine, waits for the tx to be included in the block.
// The wait will end after either the asyncTimeout has run out or the asyncCtx exits.
// If there is no error broadcasting, the asyncCallback will be called with success/failure of the wait for block inclusion.
func BroadcastTx(
	ctx context.Context, // context for tx broadcast
	logger *zap.Logger,
	cfg *config.BabylonConfig,
	encCfg *appparams.EncodingConfig,

	rpcClient *babylonclient.RPCClient,
	tx []byte, // raw tx to be broadcasted
	msgs []sdk.Msg, // used for logging only

	asyncCtx context.Context, // context for async wait for block inclusion after successful tx broadcast
	asyncTimeout time.Duration, // timeout for waiting for block inclusion
	asyncCallbacks []callbackTx, // callback for success/fail of the wait for block inclusion
) error {
	res, err := rpcClient.BroadcastTxSync(ctx, tx)
	isErr := err != nil
	isFailed := res != nil && res.Code != 0
	if isErr || isFailed {
		if isErr && res == nil {
			// There are some cases where BroadcastTxSync will return an error but the associated
			// ResultBroadcastTx will be nil.
			return err
		}
		rlyResp := &babylonclient.RelayerTxResponse{
			TxHash:    res.Hash.String(),
			Codespace: res.Codespace,
			Code:      res.Code,
			Data:      res.Data.String(),
		}
		if isFailed {
			err = sdkError(res.Codespace, res.Code)
			if err == nil {
				err = fmt.Errorf("transaction failed to execute: codespace: %s, code: %d, log: %s", res.Codespace, res.Code, res.Log)
			}
		}
		LogFailedTx(logger, cfg.ChainID, rlyResp, err, msgs)
		return err
	}

	// TODO: maybe we need to check if the node has tx indexing enabled?
	// if not, we need to find a new way to block until inclusion in a block
	protoCdc := codec.NewProtoCodec(encCfg.InterfaceRegistry)
	go waitForTx(asyncCtx, logger, rpcClient, protoCdc, encCfg.TxConfig, cfg.ChainID, res.Hash, msgs, asyncTimeout, asyncCallbacks)

	return nil
}

// waitForTx waits for a transaction to be included in a block, logs success/fail, then invokes callback.
// This is intended to be called as an async goroutine.
func waitForTx(
	ctx context.Context,
	log *zap.Logger,
	rpcClient *babylonclient.RPCClient,
	cdc *codec.ProtoCodec,
	txConfig client.TxConfig,
	chainId string,
	txHash []byte,
	msgs []sdk.Msg, // used for logging only
	waitTimeout time.Duration,
	callbacks []callbackTx,
) {
	res, err := waitForBlockInclusion(ctx, rpcClient, txConfig, txHash, waitTimeout)
	if err != nil {
		log.Error("Failed to wait for block inclusion", zap.Error(err))
		if len(callbacks) > 0 {
			for _, cb := range callbacks {
				//Call each callback in order since waitForTx is already invoked asyncronously
				cb(nil, err)
			}
		}
		return
	}

	rlyResp := &babylonclient.RelayerTxResponse{
		Height:    res.Height,
		TxHash:    res.TxHash,
		Codespace: res.Codespace,
		Code:      res.Code,
		Data:      res.Data,
		Events:    parseEventsFromTxResponse(res),
	}

	// transaction was executed, log the success or failure using the tx response code
	// NOTE: error is nil, logic should use the returned error to determine if the
	// transaction was successfully executed.

	if res.Code != 0 {
		// Check for any registered SDK errors
		err := sdkError(res.Codespace, res.Code)
		if err == nil {
			err = fmt.Errorf("transaction failed to execute: codespace: %s, code: %d, log: %s", res.Codespace, res.Code, res.RawLog)
		}
		if len(callbacks) > 0 {
			for _, cb := range callbacks {
				//Call each callback in order since waitForTx is already invoked asyncronously
				cb(nil, err)
			}
		}
		LogFailedTx(log, chainId, rlyResp, nil, msgs)
		return
	}

	if len(callbacks) > 0 {
		for _, cb := range callbacks {
			//Call each callback in order since waitForTx is already invoked asyncronously
			cb(res, nil)
		}
	}
	LogSuccessTx(log, chainId, cdc, res, msgs)
}

// waitForBlockInclusion will wait for a transaction to be included in a block, up to waitTimeout or context cancellation.
func waitForBlockInclusion(
	ctx context.Context,
	rpcClient *babylonclient.RPCClient,
	txConfig client.TxConfig,
	txHash []byte,
	waitTimeout time.Duration,
) (*sdk.TxResponse, error) {
	exitAfter := time.After(waitTimeout)
	for {
		select {
		case <-exitAfter:
			return nil, fmt.Errorf("timed out after: %d; %w", waitTimeout, babylonclient.ErrTimeoutAfterWaitingForTxBroadcast)
		// This fixed poll is fine because it's only for logging and updating prometheus metrics currently.
		case <-time.After(time.Millisecond * 100):
			res, err := rpcClient.Tx(ctx, txHash, false)
			if err == nil {
				return mkTxResult(res, txConfig)
			}
			if strings.Contains(err.Error(), "transaction indexing is disabled") {
				return nil, fmt.Errorf("cannot determine success/failure of tx because transaction indexing is disabled on rpc url")
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// mkTxResult decodes a comet transaction into an SDK TxResponse.
func mkTxResult(
	resTx *coretypes.ResultTx,
	txConfig client.TxConfig,
) (*sdk.TxResponse, error) {
	txbz, err := txConfig.TxDecoder()(resTx.Tx)
	if err != nil {
		return nil, err
	}

	p, ok := txbz.(intoAny)
	if !ok {
		return nil, fmt.Errorf("expecting a type implementing intoAny, got: %T", txbz)
	}

	any := p.AsAny()
	return sdk.NewResponseResultTx(resTx, any, ""), nil
}

func AccessKeyWithLock(keyDir string, accessFunc func() error) error {
	// use lock file to guard concurrent access to the keyring
	lockFilePath := path.Join(keyDir, "keys.lock")
	lock := fslock.New(lockFilePath)
	err := lock.Lock()
	if err != nil {
		return fmt.Errorf("failed to acquire file system lock (%s): %w", lockFilePath, err)
	}

	// trigger function that access keyring
	err = accessFunc()

	// unlock and release access
	if errUnlock := lock.Unlock(); errUnlock != nil {
		return fmt.Errorf("error unlocking file system lock (%s), please manually delete", lockFilePath)
	}

	return err
}

// QueryABCI performs an ABCI query and returns the appropriate response and error sdk error code.
func QueryABCI(ctx context.Context, rpcClient *babylonclient.RPCClient, req abci.RequestQuery) (abci.ResponseQuery, error) {
	opts := rpcclient.ABCIQueryOptions{
		Height: req.Height,
		Prove:  req.Prove,
	}

	result, err := rpcClient.ABCIQueryWithOptions(ctx, req.Path, req.Data, opts)
	if err != nil {
		return abci.ResponseQuery{}, err
	}

	if !result.Response.IsOK() {
		return abci.ResponseQuery{}, sdkErrorToGRPCError(result.Response.Code, result.Response.Log)
	}

	// data from trusted node or subspace query doesn't need verification
	if !opts.Prove || !isQueryStoreWithProof(req.Path) {
		return result.Response, nil
	}

	return result.Response, nil
}

// isQueryStoreWithProof expects a format like /<queryType>/<storeName>/<subpath>
// queryType must be "store" and subpath must be "key" to require a proof.
func isQueryStoreWithProof(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}

	paths := strings.SplitN(path[1:], "/", 3)

	switch {
	case len(paths) != 3:
		return false
	case paths[0] != "store":
		return false
	case rootmulti.RequireProof("/" + paths[2]):
		return true
	}

	return false
}

func ErrorContained(err error, errList []*sdkerrors.Error) bool {
	for _, e := range errList {
		if strings.Contains(err.Error(), e.Error()) {
			return true
		}
	}

	return false
}

func KeybaseFromCfg(
	cfg *config.BabylonConfig,
	cdc codec.Codec,
) (keyring.Keyring, error) {
	return keyring.New(
		cfg.ChainID,
		cfg.KeyringBackend,
		cfg.KeyDirectory,
		os.Stdin,
		cdc,
	)
}

// PrepareFactory mutates the tx factory with the appropriate account number, sequence number, and min gas settings.
func PrepareFactory(
	cliCtx client.Context,
	txf tx.Factory,
	keybase keyring.Keyring,
	signingKey string,
) (tx.Factory, error) {
	var (
		err      error
		from     sdk.AccAddress
		num, seq uint64
	)

	// Get key address and retry if fail
	if err = retry.Do(func() error {
		from, err = GetKeyAddressForKey(keybase, signingKey)
		if err != nil {
			return err
		}
		return err
	}, rtyAtt, rtyDel, rtyErr); err != nil {
		return tx.Factory{}, err
	}

	cliCtx = cliCtx.WithFromAddress(from)

	// TODO: why this code? this may potentially require another query when we don't want one
	initNum, initSeq := txf.AccountNumber(), txf.Sequence()
	if initNum == 0 || initSeq == 0 {
		if err = retry.Do(func() error {
			num, seq, err = txf.AccountRetriever().GetAccountNumberSequence(cliCtx, from)
			if err != nil {
				return err
			}
			return err
		}, rtyAtt, rtyDel, rtyErr); err != nil {
			return txf, err
		}

		if initNum == 0 {
			txf = txf.WithAccountNumber(num)
		}

		if initSeq == 0 {
			txf = txf.WithSequence(seq)
		}
	}

	return txf, nil
}

func GetKeyAddressForKey(keybase keyring.Keyring, key string) (sdk.AccAddress, error) {
	info, err := keybase.Key(key)
	if err != nil {
		return nil, err
	}
	return info.GetAddress()
}

// TxFactory instantiates a new tx factory with the appropriate configuration settings for this chain.
func TxFactory(
	cfg *config.BabylonConfig,
	txConf client.TxConfig,
	keybase keyring.Keyring,
) tx.Factory {
	return tx.Factory{}.
		WithChainID(cfg.ChainID).
		WithTxConfig(txConf).
		WithGasAdjustment(cfg.GasAdjustment).
		WithGasPrices(cfg.GasPrices).
		WithKeybase(keybase).
		WithSignMode(SignMode(cfg.SignModeStr))
}

func SignMode(signModeStr string) signing.SignMode {
	switch signModeStr {
	case "direct":
		return signing.SignMode_SIGN_MODE_DIRECT
	case "amino-json":
		return signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON
	default:
		return signing.SignMode_SIGN_MODE_UNSPECIFIED
	}
}

// BuildSimTx creates an unsigned tx with an empty single signature and returns
// the encoded transaction or an error if the unsigned transaction cannot be built.
func BuildSimTx(info *keyring.Record, txf tx.Factory, msgs ...sdk.Msg) ([]byte, error) {
	txb, err := txf.BuildUnsignedTx(msgs...)
	if err != nil {
		return nil, err
	}

	pk, err := info.GetPubKey()
	if err != nil {
		return nil, err
	}

	// Create an empty signature literal as the ante handler will populate with a
	// sentinel pubkey.
	sig := signing.SignatureV2{
		PubKey: pk,
		Data: &signing.SingleSignatureData{
			SignMode: txf.SignMode(),
		},
		Sequence: txf.Sequence(),
	}
	if err := txb.SetSignatures(sig); err != nil {
		return nil, err
	}

	protoProvider, ok := txb.(protoTxProvider)
	if !ok {
		return nil, fmt.Errorf("cannot simulate amino tx")
	}

	simReq := txtypes.SimulateRequest{Tx: protoProvider.GetProtoTx()}
	return simReq.Marshal()
}

// protoTxProvider is a type which can provide a proto transaction. It is a
// workaround to get access to the wrapper TxBuilder's method GetProtoTx().
type protoTxProvider interface {
	GetProtoTx() *txtypes.Tx
}

func sdkErrorToGRPCError(code uint32, log string) error {
	switch code {
	case legacyerrors.ErrInvalidRequest.ABCICode():
		return status.Error(codes.InvalidArgument, log)
	case legacyerrors.ErrUnauthorized.ABCICode():
		return status.Error(codes.Unauthenticated, log)
	case legacyerrors.ErrKeyNotFound.ABCICode():
		return status.Error(codes.NotFound, log)
	default:
		return status.Error(codes.Unknown, log)
	}
}

// AdjustEstimatedGas adjusts the estimated gas usage by multiplying it by the gas adjustment factor
// and return estimated gas is higher than max gas error. If the gas usage is zero, the adjusted gas
// is also zero.
func AdjustEstimatedGas(gasAdjustment float64, gasUsed uint64) (uint64, error) {
	if gasUsed == 0 {
		return gasUsed, nil
	}

	gas := gasAdjustment * float64(gasUsed)
	if math.IsInf(gas, 1) {
		return 0, fmt.Errorf("infinite gas used")
	}
	return uint64(gas), nil
}

// sdkError will return the Cosmos SDK registered error for a given codespace/code combo if registered, otherwise nil.
func sdkError(codespace string, code uint32) error {
	// ABCIError will return an error other than "unknown" if syncRes.Code is a registered error in syncRes.Codespace
	// This catches all of the sdk errors https://github.com/cosmos/cosmos-sdk/blob/f10f5e5974d2ecbf9efc05bc0bfe1c99fdeed4b6/types/errors/errors.go
	err := errors.Unwrap(sdkerrors.ABCIError(codespace, code, "error broadcasting transaction"))
	if err.Error() != "unknown" {
		return err
	}
	return nil
}

// Deprecated: this interface is used only internally for scenario we are
// deprecating (StdTxConfig support)
type intoAny interface {
	AsAny() *codectypes.Any
}

// CleanSlice removes nil values from a slice of pointers.
func CleanSlice[T any](slice []*T) []*T {
	result := make([]*T, 0, len(slice))
	for _, item := range slice {
		if item != nil {
			result = append(result, item)
		}
	}
	return result
}

func reliablySendEachMsgAsTxCallback(
	log *zap.Logger,
	wg *sync.WaitGroup,
	msg sdk.Msg,
	msgIndex int,
	txResponses []*sdk.TxResponse,
	failedMsgs []*failedMsg,
) callbackTx {
	return func(txResp *sdk.TxResponse, err error) {
		defer wg.Done()

		if err != nil {
			failedMsgs[msgIndex] = &failedMsg{
				msg:    msg,
				reason: err,
			}

			if ErrorContained(err, expectedErrors) {
				log.Debug(
					"sucessfully submit message, got expected error",
					zap.Int("msg_index", msgIndex),
				)
				return
			}

			log.Error(
				"failed to submit message",
				zap.Int("msg_index", msgIndex),
				zap.String("msg_data", msg.String()),
				zap.Error(err),
			)
			return
		}

		log.Debug(
			"sucessfully submit message",
			zap.Int("msg_index", msgIndex),
			zap.String("tx_hash", txResp.TxHash),
		)
		txResponses[msgIndex] = txResp
	}

}
