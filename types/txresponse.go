package types

import "github.com/babylonlabs-io/babylon/v3/client/babylonclient"

type TxResponse struct {
	TxHash string
	Events []babylonclient.RelayerEvent
}
