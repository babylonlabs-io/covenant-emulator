//nolint:revive
package types

import "github.com/babylonlabs-io/babylon/v4/client/babylonclient"

type TxResponse struct {
	TxHash string
	Events []babylonclient.RelayerEvent
}
