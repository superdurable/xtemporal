package dummy

import (
	"github.com/superdurable/xtemporal/common/namespace"
	workercommon "github.com/superdurable/xtemporal/service/worker/common"
	sdkworker "go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/fx"
)

type workerComponent struct{}

type fxResult struct {
	fx.Out
	Component workercommon.PerNSWorkerComponent `group:"perNamespaceWorkerComponent"`
}

var Module = fx.Options(fx.Provide(NewResult))

func NewResult() fxResult {
	return fxResult{
		Component: &workerComponent{},
	}
}

func (c *workerComponent) Register(registry sdkworker.Registry, ns *namespace.Namespace, _ workercommon.RegistrationDetails) func() {
	registry.RegisterWorkflowWithOptions(DummyWorkflow, workflow.RegisterOptions{Name: DummyWFTypeName})
	return nil
}

func (c *workerComponent) DedicatedWorkerOptions(ns *namespace.Namespace) *workercommon.PerNSDedicatedWorkerOptions {
	return &workercommon.PerNSDedicatedWorkerOptions{
		Enabled: true,
	}
}
