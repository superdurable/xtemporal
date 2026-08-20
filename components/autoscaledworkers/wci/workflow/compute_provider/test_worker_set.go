package computeprovider

import (
	"context"
	"errors"
	"fmt"

	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/components/autoscaledworkers/wci/workflow/iface"
)

const (
	configTestWorkerSetIllegalField = "illegal_field"
)

type testWorkerSetComputeProvider struct{}

func init() {
	RegisterComputeProvider(iface.ComputeProviderTypeTestWorkerSet, NewTestWorkerSetComputeProvider)
}

func NewTestWorkerSetComputeProvider(_ context.Context, _ *dynamicconfig.Collection) (ComputeProvider, error) {
	return &testWorkerSetComputeProvider{}, nil
}

func (p *testWorkerSetComputeProvider) LaunchStrategy() LaunchStrategy {
	return LaunchStrategyWorkerSet
}

func (p *testWorkerSetComputeProvider) ValidateConfig(ctx context.Context, config ComputeProviderConfig) error {
	if _, ok := config[configTestWorkerSetIllegalField].(string); ok {
		return fmt.Errorf("illegal_field found in config")
	}

	return nil
}

func (p *testWorkerSetComputeProvider) InvokeWorker(ctx context.Context, config ComputeProviderConfig) error {
	return errors.ErrUnsupported
}

func (p *testWorkerSetComputeProvider) UpdateWorkerSetSize(_ context.Context, _ ComputeProviderConfig, _ int32) error {
	return nil
}
