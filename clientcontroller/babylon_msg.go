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
	bbn "github.com/babylonlabs-io/babylon/app"
	"github.com/babylonlabs-io/babylon/client/config"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	legacyerrors "github.com/cosmos/cosmos-sdk/types/errors"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"github.com/juju/fslock"
	abcistrange "github.com/strangelove-ventures/cometbft-client/abci/types"
	strangeloveclient "github.com/strangelove-ventures/cometbft-client/client"
	rpcclient "github.com/strangelove-ventures/cometbft-client/rpc/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	rtyAttNum                   = uint(5)
	rtyAtt                      = retry.Attempts(rtyAttNum)
	rtyDel                      = retry.Delay(time.Millisecond * 400)
	rtyErr                      = retry.LastErrorOnly(true)
	defaultBroadcastWaitTimeout = 10 * time.Minute
	spTag                       = "send_packet"
	waTag                       = "write_acknowledgement"
	srcChanTag                  = "packet_src_channel"
	dstChanTag                  = "packet_dst_channel"
)

func reliablySendMsgsAsMultipleTxs(
	cfg *config.BabylonConfig,
	msgs []sdk.Msg,
) error {

	c, err := strangeloveclient.NewClient(cfg.RPCAddr, cfg.Timeout)
	if err != nil {
		return err
	}

	return nil
}

// ReliablySendMsgs reliably sends a list of messages to the chain.
// It utilizes a file lock as well as a keyring lock to ensure atomic access.
func ReliablySendMsgs(
	ctx context.Context,
	logger *zap.Logger,
	cfg *config.BabylonConfig,
	msgs []sdk.Msg,
	expectedErrors, unrecoverableErrors []*errors.Error,
) (*sdk.TxResponse, error) {
	var (
		rlyResp     *sdk.TxResponse
		callbackErr error
		wg          sync.WaitGroup
	)

	callback := func(rtr *sdk.TxResponse, err error) {
		rlyResp = rtr
		callbackErr = err
		wg.Done()
	}

	wg.Add(1)

	if err := retry.Do(func() error {
		var sendMsgErr error
		krErr := AccessKeyWithLock(cfg.KeyDirectory, func() {
			sendMsgErr = c.provider.SendMessagesToMempool(ctx, msgs, "", ctx, []func(*sdk.TxResponse, error){callback})
		})
		if krErr != nil {
			logger.Error("unrecoverable err when submitting the tx, skip retrying", zap.Error(krErr))
			return retry.Unrecoverable(krErr)
		}
		if sendMsgErr != nil {
			if ErrorContained(sendMsgErr, unrecoverableErrors) {
				logger.Error("unrecoverable err when submitting the tx, skip retrying", zap.Error(sendMsgErr))
				return retry.Unrecoverable(sendMsgErr)
			}
			if ErrorContained(sendMsgErr, expectedErrors) {
				// this is necessary because if err is returned
				// the callback function will not be executed so
				// that the inside wg.Done will not be executed
				wg.Done()
				logger.Error("expected err when submitting the tx, skip retrying", zap.Error(sendMsgErr))
				return nil
			}
			return sendMsgErr
		}
		return nil
	}, retry.Context(ctx), rtyAtt, rtyDel, rtyErr, retry.OnRetry(func(n uint, err error) {
		logger.Debug("retrying", zap.Uint("attempt", n+1), zap.Uint("max_attempts", rtyAttNum), zap.Error(err))
	})); err != nil {
		return nil, err
	}

	wg.Wait()

	if callbackErr != nil {
		if ErrorContained(callbackErr, expectedErrors) {
			return nil, nil
		}
		return nil, callbackErr
	}

	if rlyResp == nil {
		// this case could happen if the error within the retry is an expected error
		return nil, nil
	}

	if rlyResp.Code != 0 {
		return rlyResp, fmt.Errorf("transaction failed with code: %d", rlyResp.Code)
	}

	return rlyResp, nil
}

// SendMessagesToMempool simulates and broadcasts a transaction with the given msgs and memo.
// This method will return once the transaction has entered the mempool.
// In an async goroutine, will wait for the tx to be included in the block unless asyncCtx exits.
// If there is no error broadcasting, the asyncCallback will be called with success/failure of the wait for block inclusion.
func SendMessagesToMempool(
	ctx context.Context,
	cfg *config.BabylonConfig,
	cometClient client.CometRPC,
	rpcClient *strangeloveclient.Client,
	ar client.AccountRetriever,

	msgs []sdk.Msg,
	memo string,

	txSignerKey string,
	sequence uint64,

	asyncCtx context.Context,
	asyncCallbacks []func(sdk.TxResponse, error),
) error {

	txBytes, fees, err := BuildMessages(
		ctx, cfg, cometClient, rpcClient, ar, msgs, memo, 0, txSignerKey, sequence,
	)
	if err != nil {
		return err
	}

	if err := cc.broadcastTx(ctx, txBytes, msgs, fees, asyncCtx, defaultBroadcastWaitTimeout, asyncCallbacks); err != nil {
		return err
	}

	return nil
}

func BuildMessages(
	ctx context.Context,
	cfg *config.BabylonConfig,
	cometClient client.CometRPC,
	rpcClient *strangeloveclient.Client,
	ar client.AccountRetriever,
	msgs []sdk.Msg,
	memo string,
	gas uint64,
	txSignerKey string,
	sequence uint64,
) (
	txBytes []byte,
	fees sdk.Coins,
	err error,
) {
	encCfg := bbn.GetEncodingConfig()

	keybase, err := keyring.New(
		cfg.ChainID,
		cfg.KeyringBackend,
		cfg.KeyDirectory,
		os.Stdin,
		encCfg.Codec,
	)
	if err != nil {
		return nil, sdk.Coins{}, err
	}

	cliCtx := client.Context{}.WithClient(cometClient).
		WithInterfaceRegistry(encCfg.InterfaceRegistry).
		WithChainID(cfg.ChainID).
		WithCodec(encCfg.Codec)

	txf := TxFactory(cfg, ar, encCfg.TxConfig, keybase)
	txf, err = PrepareFactory(cliCtx, txf, keybase, txSignerKey)
	if err != nil {
		return nil, sdk.Coins{}, err
	}

	if memo != "" {
		txf = txf.WithMemo(memo)
	}

	txf = txf.WithSequence(sequence)

	adjusted := gas

	if gas == 0 {
		_, adjusted, err = CalculateGas(ctx, rpcClient, keybase, txf, txSignerKey, cfg.GasAdjustment, msgs...)

		if err != nil {
			return nil, sdk.Coins{}, err
		}
	}

	// Set the gas amount on the transaction factory
	txf = txf.WithGas(adjusted)

	// Build the transaction builder
	txb, err := txf.BuildUnsignedTx(msgs...)
	if err != nil {
		return nil, sdk.Coins{}, err
	}

	if err = tx.Sign(ctx, txf, txSignerKey, txb, false); err != nil {
		return nil, sdk.Coins{}, err
	}

	tx := txb.GetTx()
	fees = tx.GetFee()

	// Generate the transaction bytes
	txBytes, err = encCfg.TxConfig.TxEncoder()(tx)
	if err != nil {
		return nil, sdk.Coins{}, err
	}

	return txBytes, fees, nil
}

// BroadcastTx broadcasts a transaction with the given raw bytes and then, in an async goroutine, waits for the tx to be included in the block.
// The wait will end after either the asyncTimeout has run out or the asyncCtx exits.
// If there is no error broadcasting, the asyncCallback will be called with success/failure of the wait for block inclusion.
func BroadcastTx(
	ctx context.Context, // context for tx broadcast
	logger *zap.Logger,
	cfg *config.BabylonConfig,

	rpcClient *strangeloveclient.Client,
	tx []byte, // raw tx to be broadcasted
	msgs []provider.RelayerMessage, // used for logging only
	fees sdk.Coins, // used for metrics

	asyncCtx context.Context, // context for async wait for block inclusion after successful tx broadcast
	asyncTimeout time.Duration, // timeout for waiting for block inclusion
	asyncCallbacks []func(*provider.RelayerTxResponse, error), // callback for success/fail of the wait for block inclusion
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
		rlyResp := &provider.RelayerTxResponse{
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

	go cc.waitForTx(asyncCtx, res.Hash, msgs, asyncTimeout, asyncCallbacks)

	return nil
}

// CalculateGas simulates a tx to generate the appropriate gas settings before broadcasting a tx.
func CalculateGas(
	ctx context.Context,
	rpcClient *strangeloveclient.Client,
	keybase keyring.Keyring,
	txf tx.Factory,
	signingKey string,
	gasAdjustment float64,
	msgs ...sdk.Msg,
) (txtypes.SimulateResponse, uint64, error) {
	keyInfo, err := keybase.Key(signingKey)
	if err != nil {
		return txtypes.SimulateResponse{}, 0, err
	}

	var txBytes []byte
	if err := retry.Do(func() error {
		var err error
		txBytes, err = BuildSimTx(keyInfo, txf, msgs...)
		if err != nil {
			return err
		}
		return nil
	}, retry.Context(ctx), rtyAtt, rtyDel, rtyErr); err != nil {
		return txtypes.SimulateResponse{}, 0, err
	}

	simQuery := abci.RequestQuery{
		Path: "/cosmos.tx.v1beta1.Service/Simulate",
		Data: txBytes,
	}

	var res abcistrange.ResponseQuery
	if err := retry.Do(func() error {
		var err error
		res, err = QueryABCI(ctx, rpcClient, simQuery)
		if err != nil {
			return err
		}
		return nil
	}, retry.Context(ctx), rtyAtt, rtyDel, rtyErr); err != nil {
		return txtypes.SimulateResponse{}, 0, err
	}

	var simRes txtypes.SimulateResponse
	if err := simRes.Unmarshal(res.Value); err != nil {
		return txtypes.SimulateResponse{}, 0, err
	}

	gas, err := AdjustEstimatedGas(gasAdjustment, simRes.GasInfo.GasUsed)
	return simRes, gas, err
}

// waitForTx waits for a transaction to be included in a block, logs success/fail, then invokes callback.
// This is intended to be called as an async goroutine.
func waitForTx(
	ctx context.Context,
	log *zap.Logger,
	txHash []byte,
	msgs []provider.RelayerMessage, // used for logging only
	waitTimeout time.Duration,
	callbacks []func(*provider.RelayerTxResponse, error),
) {
	res, err := cc.waitForBlockInclusion(ctx, txHash, waitTimeout)
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

	rlyResp := &provider.RelayerTxResponse{
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
		err := cc.sdkError(res.Codespace, res.Code)
		if err == nil {
			err = fmt.Errorf("transaction failed to execute: codespace: %s, code: %d, log: %s", res.Codespace, res.Code, res.RawLog)
		}
		if len(callbacks) > 0 {
			for _, cb := range callbacks {
				//Call each callback in order since waitForTx is already invoked asyncronously
				cb(nil, err)
			}
		}
		LogFailedTx(log, rlyResp, nil, msgs)
		return
	}

	if len(callbacks) > 0 {
		for _, cb := range callbacks {
			//Call each callback in order since waitForTx is already invoked asyncronously
			cb(rlyResp, nil)
		}
	}
	cc.LogSuccessTx(res, msgs)
}

func AccessKeyWithLock(keyDir string, accessFunc func()) error {
	// use lock file to guard concurrent access to the keyring
	lockFilePath := path.Join(keyDir, "keys.lock")
	lock := fslock.New(lockFilePath)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("failed to acquire file system lock (%s): %w", lockFilePath, err)
	}

	// trigger function that access keyring
	accessFunc()

	// unlock and release access
	if err := lock.Unlock(); err != nil {
		return fmt.Errorf("error unlocking file system lock (%s), please manually delete", lockFilePath)
	}

	return nil
}

// QueryABCI performs an ABCI query and returns the appropriate response and error sdk error code.
func QueryABCI(ctx context.Context, rpcClient *strangeloveclient.Client, req abci.RequestQuery) (abcistrange.ResponseQuery, error) {
	opts := rpcclient.ABCIQueryOptions{
		Height: req.Height,
		Prove:  req.Prove,
	}

	result, err := rpcClient.ABCIQueryWithOptions(ctx, req.Path, req.Data, opts)
	if err != nil {
		return abcistrange.ResponseQuery{}, err
	}

	if !result.Response.IsOK() {
		return abcistrange.ResponseQuery{}, sdkErrorToGRPCError(result.Response.Code, result.Response.Log)
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

	// Set the account number and sequence on the transaction factory and retry if fail
	if err = retry.Do(func() error {
		if err = txf.AccountRetriever().EnsureExists(cliCtx, from); err != nil {
			return err
		}
		return err
	}, rtyAtt, rtyDel, rtyErr); err != nil {
		return txf, err
	}

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
	ar client.AccountRetriever,
	txConf client.TxConfig,
	keybase keyring.Keyring,
) tx.Factory {
	return tx.Factory{}.
		WithAccountRetriever(ar).
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

	var pk cryptotypes.PubKey = &secp256k1.PubKey{} // use default public key type

	pk, err = info.GetPubKey()
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

// LogFailedTx takes the transaction and the messages to create it and logs the appropriate data
func LogFailedTx(log *zap.Logger, chainId string, res *provider.RelayerTxResponse, err error, msgs []provider.RelayerMessage) {
	// Include the chain_id
	fields := []zapcore.Field{zap.String("chain_id", chainId)}

	// Extract the channels from the events, if present
	if res != nil {
		channels := getChannelsIfPresent(res.Events)
		fields = append(fields, channels...)
	}
	fields = append(fields, msgTypesField(msgs))

	if err != nil {

		if errors.Is(err, chantypes.ErrRedundantTx) {
			log.Debug("Redundant message(s)", fields...)
			return
		}

		// Make a copy since we may continue to the warning
		errorFields := append(fields, zap.Error(err))
		log.Error(
			"Failed sending cosmos transaction",
			errorFields...,
		)

		if res == nil {
			return
		}
	}

	if res.Code != 0 {
		if sdkErr := cc.sdkError(res.Codespace, res.Code); err != nil {
			fields = append(fields, zap.NamedError("sdk_error", sdkErr))
		}
		fields = append(fields, zap.Object("response", res))
		cc.log.Warn(
			"Sent transaction but received failure response",
			fields...,
		)
	}
}

// getChannelsIfPresent scans the events for channel tags
func getChannelsIfPresent(events []provider.RelayerEvent) []zapcore.Field {
	channelTags := []string{srcChanTag, dstChanTag}
	fields := []zap.Field{}

	// While a transaction may have multiple messages, we just need to first
	// pair of channels
	foundTag := map[string]struct{}{}

	for _, event := range events {
		for _, tag := range channelTags {
			for attributeKey, attributeValue := range event.Attributes {
				if attributeKey == tag {
					// Only append the tag once
					// TODO: what if they are different?
					if _, ok := foundTag[tag]; !ok {
						fields = append(fields, zap.String(tag, attributeValue))
						foundTag[tag] = struct{}{}
					}
				}
			}
		}
	}
	return fields
}

func msgTypesField(msgs []provider.RelayerMessage) zap.Field {
	msgTypes := make([]string, len(msgs))
	for i, m := range msgs {
		msgTypes[i] = m.Type()
	}
	return zap.Strings("msg_types", msgTypes)
}
