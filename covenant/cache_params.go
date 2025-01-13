package covenant

import (
	"sync"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/covenant-emulator/clientcontroller"
	"github.com/babylonlabs-io/covenant-emulator/types"
	"go.uber.org/zap"
)

type (
	ParamsGetter interface {
		Get(version uint32) (*types.StakingParams, error)
	}
	CacheVersionedParams struct {
		sync.Mutex
		paramsByVersion map[uint32]*types.StakingParams

		cc     clientcontroller.ClientController
		logger *zap.Logger
	}
)

func NewCacheVersionedParams(cc clientcontroller.ClientController, logger *zap.Logger) ParamsGetter {
	return &CacheVersionedParams{
		paramsByVersion: make(map[uint32]*types.StakingParams),
		cc:              cc,
		logger:          logger,
	}
}

// Get returns the staking parameter from the
func (v *CacheVersionedParams) Get(version uint32) (*types.StakingParams, error) {
	v.Lock()
	defer v.Unlock()

	params, ok := v.paramsByVersion[version]
	if ok {
		return params, nil
	}

	params, err := v.getParamsByVersion(version)
	if err != nil {
		return nil, err
	}

	v.paramsByVersion[version] = params
	return params, nil
}

func (v *CacheVersionedParams) getParamsByVersion(version uint32) (*types.StakingParams, error) {
	var (
		err    error
		params *types.StakingParams
	)

	if err := retry.Do(func() error {
		params, err = v.cc.QueryStakingParamsByVersion(version)
		if err != nil {
			return err
		}
		return nil
	}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
		v.logger.Debug(
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
