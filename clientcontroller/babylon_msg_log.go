package clientcontroller

import (
	"errors"
	"reflect"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	typestx "github.com/cosmos/cosmos-sdk/types/tx"
	feetypes "github.com/cosmos/ibc-go/v8/modules/apps/29-fee/types"
	transfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogFailedTx takes the transaction and the messages to create it and logs the appropriate data
func LogFailedTx(log *zap.Logger, chainId string, res *provider.RelayerTxResponse, err error, msgs []sdk.Msg) {
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
		if sdkErr := sdkError(res.Codespace, res.Code); err != nil {
			fields = append(fields, zap.NamedError("sdk_error", sdkErr))
		}
		fields = append(fields, zap.Object("response", res))
		log.Warn(
			"Sent transaction but received failure response",
			fields...,
		)
	}
}

// LogSuccessTx take the transaction and the messages to create it and logs the appropriate data
func LogSuccessTx(log *zap.Logger, chainId string, cdc *codec.ProtoCodec, res *sdk.TxResponse, msgs []sdk.Msg) {
	// Include the chain_id
	fields := []zapcore.Field{zap.String("chain_id", chainId)}

	// Extract the channels from the events, if present
	if res != nil {
		events := parseEventsFromTxResponse(res)
		fields = append(fields, getChannelsIfPresent(events)...)
	}

	// Include the gas used
	fields = append(fields, zap.Int64("gas_used", res.GasUsed))

	var m sdk.Msg
	if err := cdc.InterfaceRegistry().UnpackAny(res.Tx, &m); err == nil {
		if tx, ok := m.(*typestx.Tx); ok {
			fields = append(fields, zap.Stringer("fees", tx.GetFee()))
			if feePayer := getFeePayer(log, cdc, tx); feePayer != "" {
				fields = append(fields, zap.String("fee_payer", feePayer))
			}
		} else {
			log.Debug(
				"Failed to convert message to Tx type",
				zap.Stringer("type", reflect.TypeOf(m)),
			)
		}
	} else {
		log.Debug("Failed to unpack response Tx into sdk.Msg", zap.Error(err))
	}

	// Include the height, msgType, and tx_hash
	fields = append(fields,
		zap.Int64("height", res.Height),
		msgTypesField(msgs),
		zap.String("tx_hash", res.TxHash),
	)

	// Log the successful transaction with fields
	log.Info(
		"Successful transaction",
		fields...,
	)
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

func msgTypesField(msgs []sdk.Msg) zap.Field {
	msgTypes := make([]string, len(msgs))
	for i, m := range msgs {
		msgTypes[i] = sdk.MsgTypeURL(m)
	}
	return zap.Strings("msg_types", msgTypes)
}

// getFeePayer returns the bech32 address of the fee payer of a transaction.
// This uses the fee payer field if set,
// otherwise falls back to the address of whoever signed the first message.
func getFeePayer(log *zap.Logger, cdc *codec.ProtoCodec, tx *typestx.Tx) string {
	payer := tx.AuthInfo.Fee.Payer
	if payer != "" {
		return payer
	}

	switch firstMsg := tx.GetMsgs()[0].(type) {
	case *transfertypes.MsgTransfer:
		// There is a possible data race around concurrent map access
		// in the cosmos sdk when it converts the address from bech32.
		// We don't need the address conversion; just the sender is all that
		// GetSigners is doing under the hood anyway.
		return firstMsg.Sender
	case *clienttypes.MsgCreateClient:
		// Without this particular special case, there is a panic in ibc-go
		// due to the sdk config singleton expecting one bech32 prefix but seeing another.
		return firstMsg.Signer
	case *clienttypes.MsgUpdateClient:
		// Same failure mode as MsgCreateClient.
		return firstMsg.Signer
	case *clienttypes.MsgUpgradeClient:
		return firstMsg.Signer
	case *clienttypes.MsgSubmitMisbehaviour:
		// Same failure mode as MsgCreateClient.
		return firstMsg.Signer
	case *feetypes.MsgRegisterPayee:
		return firstMsg.Relayer
	case *feetypes.MsgRegisterCounterpartyPayee:
		return firstMsg.Relayer
	case *feetypes.MsgPayPacketFee:
		return firstMsg.Signer
	case *feetypes.MsgPayPacketFeeAsync:
		return firstMsg.PacketFee.RefundAddress
	default:
		signers, _, err := cdc.GetMsgV1Signers(firstMsg)
		if err != nil {
			log.Info("Could not get signers for msg when attempting to get the fee payer", zap.Error(err))
			return ""
		}

		return string(signers[0])
	}
}

func parseEventsFromTxResponse(resp *sdk.TxResponse) []provider.RelayerEvent {
	var events []provider.RelayerEvent

	if resp == nil {
		return events
	}

	for _, logs := range resp.Logs {
		for _, event := range logs.Events {
			attributes := make(map[string]string)
			for _, attribute := range event.Attributes {
				attributes[attribute.Key] = attribute.Value
			}
			events = append(events, provider.RelayerEvent{
				EventType:  event.Type,
				Attributes: attributes,
			})
		}
	}

	// After SDK v0.50, indexed events are no longer provided in the logs on
	// transaction execution, the response events can be directly used
	if len(events) == 0 {
		for _, event := range resp.Events {
			attributes := make(map[string]string)
			for _, attribute := range event.Attributes {
				attributes[attribute.Key] = attribute.Value
			}
			events = append(events, provider.RelayerEvent{
				EventType:  event.Type,
				Attributes: attributes,
			})
		}
	}

	return events
}
