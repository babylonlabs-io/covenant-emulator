package clientcontroller

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	sdkerrors "cosmossdk.io/errors"
	"github.com/avast/retry-go/v4"
	appparams "github.com/babylonlabs-io/babylon/app/params"
	"github.com/babylonlabs-io/babylon/client/babylonclient"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	"github.com/babylonlabs-io/babylon/client/config"
	"github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/juju/fslock"

	"go.uber.org/zap"
)

// Note: most of the functions were adappt from
// https://github.com/babylonlabs-io/babylon/blob/cd0bbcd98be5e4dda081f7330140cf9dbee4c94d/client/client/tx.go#L76
var (
	rtyAttNum                   = uint(5)
	rtyAtt                      = retry.Attempts(rtyAttNum)
	rtyDel                      = retry.Delay(time.Millisecond * 400)
	rtyErr                      = retry.LastErrorOnly(true)
	defaultBroadcastWaitTimeout = 10 * time.Minute
)

// callbackTx is the expected type that waits for the inclusion of a transaction on the chain to be called
type callbackTx func(*babylonclient.RelayerTxResponse, error)

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
) (txResponses []*babylonclient.RelayerTxResponse, failedMsgs []*failedMsg, err error) {
	c, err := http.NewWithTimeout(cfg.RPCAddr, "/websocket", uint(cfg.Timeout.Seconds()))
	if err != nil {
		return nil, nil, err
	}
	rpcClient := babylonclient.NewRPCClient(c)

	ctx := context.Background()

	msgLen := len(msgs)
	// create outputs at msg len capacity to handle each msg in parallel
	// as it is easier than pass 2 channels for each func
	txResponses = make([]*babylonclient.RelayerTxResponse, msgLen)
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

	asyncCallbacks ...func(*babylonclient.RelayerTxResponse, error),
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

	asyncCallbacks ...func(*babylonclient.RelayerTxResponse, error),
) error {
	memo, gas := "", uint64(0)

	txBytes, _, err := BuildMessages(ctx, cfg, cp, encCfg, msgs, accSequence, accNumber, memo, gas)
	if err != nil {
		return err
	}

	err = cp.BroadcastTx(ctx, txBytes, ctx, defaultBroadcastWaitTimeout, asyncCallbacks)
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
	txf := TxFactory(cfg, encCfg.TxConfig).
		WithSequence(accSequence).
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

	return txBytes, fees, nil
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

// TxFactory instantiates a new tx factory with the appropriate configuration settings for this chain.
func TxFactory(
	cfg *config.BabylonConfig,
	txConf client.TxConfig,
) tx.Factory {
	return tx.Factory{}.
		WithChainID(cfg.ChainID).
		WithTxConfig(txConf).
		WithGasAdjustment(cfg.GasAdjustment).
		WithGasPrices(cfg.GasPrices).
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
	txResponses []*babylonclient.RelayerTxResponse,
	failedMsgs []*failedMsg,
) callbackTx {
	return func(txResp *babylonclient.RelayerTxResponse, err error) {
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
