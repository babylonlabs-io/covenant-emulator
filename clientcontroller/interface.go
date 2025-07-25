package clientcontroller

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/covenant-emulator/config"
	"github.com/babylonlabs-io/covenant-emulator/types"
)

const (
	babylonConsumerChainName = "babylon"
)

type (
	FilterFn         func(del *types.Delegation) (accept bool, err error)
	ClientController interface {
		// SubmitCovenantSigs submits Covenant signatures to the consumer chain, each corresponding to
		// a finality provider that the delegation is (re-)staked to
		// it returns tx hash and error
		SubmitCovenantSigs(covSigMsgs []*types.CovenantSigs) (*types.TxResponse, error)

		// QueryPendingDelegations queries BTC delegations that are in status of pending
		QueryPendingDelegations(limit uint64, filter FilterFn) ([]*types.Delegation, error)

		// QueryBTCDelegation queries the BTC delegation by the tx hash
		QueryBTCDelegation(stakingTxHashHex string) (*types.Delegation, error)

		QueryStakingParamsByVersion(version uint32) (*types.StakingParams, error)

		Close() error
	}
)

func NewClientController(chainName string, bbnConfig *config.BBNConfig, netParams *chaincfg.Params, logger *zap.Logger) (ClientController, error) {
	var (
		cc  ClientController
		err error
	)
	switch chainName {
	case babylonConsumerChainName:
		cc, err = NewBabylonController(bbnConfig, netParams, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create Babylon rpc client: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported consumer chain")
	}

	return cc, err
}
