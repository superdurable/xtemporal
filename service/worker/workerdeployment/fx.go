package workerdeployment

import (
	"time"

	deploymentspb "github.com/superdurable/temporal-server/api/deployment/v1"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/persistence/visibility/manager"
	"github.com/superdurable/temporal-server/common/resource"
	"github.com/superdurable/temporal-server/common/sdk"
	"github.com/superdurable/temporal-server/common/testing/testhooks"
	wciclient "github.com/superdurable/temporal-server/components/autoscaledworkers/wci/client"
	workercommon "github.com/superdurable/temporal-server/service/worker/common"
	sdkworker "go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/fx"
)

type DeploymentWorkflowVersion int64

const (
	// Versions of workflow logic. When introducing a new version, consider generating a new
	// history for TestReplays using generate_history.sh.

	// Represents the state before the versioning API's received the option of becoming async in nature
	InitialVersion DeploymentWorkflowVersion = iota
	// SetCurrent and SetRamping and DeleteVersion APIs are async
	AsyncSetCurrentAndRamping
	// Version Data has its own revision number with TaskQueue registration being async as well
	VersionDataRevisionNumber
)

type (
	workerComponent struct {
		activityDeps  activityDeps
		dynamicConfig *dynamicconfig.Collection
	}

	activityDeps struct {
		fx.In
		MetricsHandler                 metrics.Handler
		Logger                         log.Logger
		ClientFactory                  sdk.ClientFactory
		MatchingClient                 resource.MatchingClient
		HistoryClient                  resource.HistoryClient
		WorkerDeploymentClient         Client
		WorkerControllerInstanceClient wciclient.Client
	}

	fxResult struct {
		fx.Out
		Component workercommon.PerNSWorkerComponent `group:"perNamespaceWorkerComponent"`
	}
)

var ClientModule = fx.Options(
	wciclient.Module,
	fx.Provide(ClientProvider),
)

var Module = fx.Options(
	ClientModule,
	fx.Provide(NewResult),
)

func ClientProvider(
	logger log.Logger,
	historyClient resource.HistoryClient,
	matchingClient resource.MatchingClient,
	visibilityManager manager.VisibilityManager,
	workerControllerInstanceClient wciclient.Client,
	dc *dynamicconfig.Collection,
	testHooks testhooks.TestHooks,
	metricsHandler metrics.Handler,
) Client {
	return &ClientImpl{
		logger:                           logger,
		historyClient:                    historyClient,
		visibilityManager:                visibilityManager,
		matchingClient:                   matchingClient,
		workerControllerInstanceClient:   workerControllerInstanceClient,
		maxIDLengthLimit:                 dynamicconfig.MaxIDLengthLimit.Get(dc),
		visibilityMaxPageSize:            dynamicconfig.FrontendVisibilityMaxPageSize.Get(dc),
		maxTaskQueuesInDeploymentVersion: dynamicconfig.MatchingMaxTaskQueuesInDeploymentVersion.Get(dc),
		maxDeployments:                   dynamicconfig.MatchingMaxDeployments.Get(dc),
		testHooks:                        testHooks,
		metricsHandler:                   metricsHandler,
	}
}

func NewResult(
	dc *dynamicconfig.Collection,
	params activityDeps,
) fxResult {
	return fxResult{
		Component: &workerComponent{
			activityDeps:  params,
			dynamicConfig: dc,
		},
	}
}

func (s *workerComponent) DedicatedWorkerOptions(ns *namespace.Namespace) *workercommon.PerNSDedicatedWorkerOptions {
	return &workercommon.PerNSDedicatedWorkerOptions{
		Enabled: true,
	}
}

func (s *workerComponent) Register(registry sdkworker.Registry, ns *namespace.Namespace, details workercommon.RegistrationDetails) func() {
	workflowVersionGetter := func() DeploymentWorkflowVersion {
		val := DeploymentWorkflowVersion(dynamicconfig.MatchingDeploymentWorkflowVersion.Get(s.dynamicConfig)(ns.Name().String()))
		return val
	}

	versionWorkflow := func(ctx workflow.Context, args *deploymentspb.WorkerDeploymentVersionWorkflowArgs) error {
		refreshIntervalGetter := func() time.Duration {
			return dynamicconfig.VersionDrainageStatusRefreshInterval.Get(s.dynamicConfig)(ns.Name().String())
		}
		visibilityGracePeriodGetter := func() time.Duration {
			return dynamicconfig.VersionDrainageStatusVisibilityGracePeriod.Get(s.dynamicConfig)(ns.Name().String())
		}
		return VersionWorkflow(ctx, workflowVersionGetter, refreshIntervalGetter, visibilityGracePeriodGetter, args)
	}
	registry.RegisterWorkflowWithOptions(versionWorkflow, workflow.RegisterOptions{Name: WorkerDeploymentVersionWorkflowType})

	deploymentWorkflow := func(ctx workflow.Context, args *deploymentspb.WorkerDeploymentWorkflowArgs) error {
		maxVersionsGetter := func() int {
			return dynamicconfig.MatchingMaxVersionsInDeployment.Get(s.dynamicConfig)(ns.Name().String())
		}
		return Workflow(ctx, workflowVersionGetter, maxVersionsGetter, args)
	}
	registry.RegisterWorkflowWithOptions(deploymentWorkflow, workflow.RegisterOptions{Name: WorkerDeploymentWorkflowType})

	versionActivities := &VersionActivities{
		activityDeps: s.activityDeps,
		namespace:    ns,
	}
	registry.RegisterActivity(versionActivities)

	activities := &Activities{
		activityDeps: s.activityDeps,
		namespace:    ns,
	}
	registry.RegisterActivity(activities)
	return nil
}
