package workercomponent

import (
	"time"

	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/sdk"
	"github.com/superdurable/xtemporal/components/autoscaledworkers/wci/client"
	instancewf "github.com/superdurable/xtemporal/components/autoscaledworkers/wci/workflow"
	"github.com/superdurable/xtemporal/components/autoscaledworkers/wci/workflow/iface"
	workercommon "github.com/superdurable/xtemporal/service/worker/common"
	sdkclient "go.temporal.io/sdk/client"
	sdkworker "go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

type (
	workerComponent struct {
		dynamicConfig    *dynamicconfig.Collection
		sdkClientFactory sdk.ClientFactory
	}
)

func NewWCIPerNSWorkerComponent(dc *dynamicconfig.Collection, sdkClientFactory sdk.ClientFactory) workercommon.PerNSWorkerComponent {
	return &workerComponent{dynamicConfig: dc, sdkClientFactory: sdkClientFactory}
}

func (s *workerComponent) DedicatedWorkerOptions(ns *namespace.Namespace) *workercommon.PerNSDedicatedWorkerOptions {
	return &workercommon.PerNSDedicatedWorkerOptions{
		Enabled: true,
	}
}

func (s *workerComponent) Register(registry sdkworker.Registry, ns *namespace.Namespace, details workercommon.RegistrationDetails) func() {
	sdkClient := s.sdkClientFactory.NewClient(sdkclient.Options{
		Namespace:     ns.Name().String(),
		DataConverter: sdk.PreferProtoDataConverter,
	})

	activities := instancewf.NewActivities(ns, s.dynamicConfig, sdkClient.WorkflowService())
	versionWorkflow := func(ctx workflow.Context, args *iface.WorkerControllerInstanceWorkflowArgs) error {
		workflowVersionGetter := func() instancewf.WorkerControllerInstanceWorkflowVersion {
			return instancewf.WorkerControllerInstanceWorkflowVersion(client.WorkerControllerInstanceWorkflowVersion.Get(s.dynamicConfig)(ns.Name().String()))
		}
		maxVersionsGetter := func() int {
			return client.WorkerControllerMaxInstances.Get(s.dynamicConfig)(ns.Name().String())
		}
		validationIntervalGetter := func() time.Duration {
			sec := client.WorkerControllerPeriodicValidationIntervalSeconds.Get(s.dynamicConfig)()
			return time.Duration(sec) * time.Second
		}
		return instancewf.Workflow(ctx, workflowVersionGetter, maxVersionsGetter, validationIntervalGetter, args, activities)
	}
	registry.RegisterWorkflowWithOptions(versionWorkflow, workflow.RegisterOptions{Name: iface.WorkerControllerInstanceWorkflowType})
	validateWorkflow := func(ctx workflow.Context, args *iface.ValidateWorkerControllerInstanceSpecWorkflowArgs) error {
		return instancewf.ValidateSpecWorkflow(ctx, args, activities)
	}
	registry.RegisterWorkflowWithOptions(validateWorkflow, workflow.RegisterOptions{Name: iface.WorkerControllerInstanceValidateWorkflowType})
	registry.RegisterActivity(activities)

	return func() { sdkClient.Close() }
}
