package covenant

import (
	"sync"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/covenant-emulator/clientcontroller"
	"github.com/babylonlabs-io/covenant-emulator/types"
	"go.uber.org/zap"
)

type (
	getParamByVersion func(version uint32) (*types.StakingParams, error)
	ParamsGetter      interface {
		Get(version uint32) (*types.StakingParams, error)
	}
	CacheVersionedParams struct {
		sync.Mutex
		paramsByVersion map[uint32]*types.StakingParams

		getParamsByVersion getParamByVersion
	}
)

func NewCacheVersionedParams(f getParamByVersion) ParamsGetter {
	return &CacheVersionedParams{
		paramsByVersion:    make(map[uint32]*types.StakingParams),
		getParamsByVersion: f,
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

func paramsByVersion(
	cc clientcontroller.ClientController,
	logger *zap.Logger,
) getParamByVersion {
	return func(version uint32) (*types.StakingParams, error) {
		var (
			err    error
			params *types.StakingParams
		)

		if err := retry.Do(func() error {
			params, err = cc.QueryStakingParamsByVersion(version)
			if err != nil {
				return err
			}
			return nil
		}, RtyAtt, RtyDel, RtyErr, retry.OnRetry(func(n uint, err error) {
			logger.Debug(
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
}
