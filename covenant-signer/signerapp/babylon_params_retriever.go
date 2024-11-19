package signerapp

import (
	"context"
	"fmt"

	"github.com/babylonlabs-io/networks/parameters/parser"
)

type VersionedParamsRetriever struct {
	*parser.ParsedGlobalParams
}

var _ BabylonParamsRetriever = &VersionedParamsRetriever{}

func NewVersionedParamsRetriever(path string) (*VersionedParamsRetriever, error) {
	parsedGlobalParams, err := parser.NewParsedGlobalParamsFromFile(path)
	if err != nil {
		return nil, err
	}
	return &VersionedParamsRetriever{parsedGlobalParams}, nil
}

func (v *VersionedParamsRetriever) ParamsByHeight(ctx context.Context, height uint64) (*BabylonParams, error) {
	versionedParams := v.ParsedGlobalParams.GetVersionedGlobalParamsByHeight(height)

	if versionedParams == nil {
		return nil, fmt.Errorf("no global params for height %d", height)
	}

	return &BabylonParams{
		CovenantPublicKeys: versionedParams.CovenantPks,
		CovenantQuorum:     versionedParams.CovenantQuorum,
		MagicBytes:         versionedParams.Tag,
		UnbondingTime:      versionedParams.UnbondingTime,
		UnbondingFee:       versionedParams.UnbondingFee,
		MaxStakingAmount:   versionedParams.MaxStakingAmount,
		MinStakingAmount:   versionedParams.MinStakingAmount,
		MaxStakingTime:     versionedParams.MaxStakingTime,
		MinStakingTime:     versionedParams.MinStakingTime,
		ConfirmationDepth:  versionedParams.ConfirmationDepth,
	}, nil
}
