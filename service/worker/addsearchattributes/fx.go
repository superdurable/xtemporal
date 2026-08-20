package addsearchattributes

import (
	"context"

	"github.com/superdurable/temporal-server/common/headers"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	esclient "github.com/superdurable/temporal-server/common/persistence/visibility/store/elasticsearch/client"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/common/searchattribute"
	workercommon "github.com/superdurable/temporal-server/service/worker/common"
	sdkworker "go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/fx"
)

type (
	// addSearchAttributes represent background work needed for adding search attributes
	addSearchAttributes struct {
		initParams
	}

	initParams struct {
		fx.In
		EsClient       esclient.Client
		Manager        searchattribute.Manager
		MetricsHandler metrics.Handler
		Logger         log.Logger
	}
)

var Module = workercommon.AnnotateWorkerComponentProvider(newComponent)

func newComponent(params initParams) workercommon.WorkerComponent {
	return &addSearchAttributes{initParams: params}
}

func (wc *addSearchAttributes) RegisterWorkflow(registry sdkworker.Registry) {
	registry.RegisterWorkflowWithOptions(AddSearchAttributesWorkflow, workflow.RegisterOptions{Name: WorkflowName})
}

func (wc *addSearchAttributes) DedicatedWorkflowWorkerOptions() *workercommon.DedicatedWorkerOptions {
	// use default worker
	return nil
}

func (wc *addSearchAttributes) RegisterActivities(registry sdkworker.Registry) {
	registry.RegisterActivity(wc.activities())
}

func (wc *addSearchAttributes) DedicatedActivityWorkerOptions() *workercommon.DedicatedWorkerOptions {
	return &workercommon.DedicatedWorkerOptions{
		TaskQueue: primitives.AddSearchAttributesActivityTQ,
		Options: sdkworker.Options{
			BackgroundActivityContext: headers.SetCallerType(context.Background(), headers.CallerTypeAPI),
		},
	}
}

func (wc *addSearchAttributes) activities() *activities {
	return &activities{
		esClient:       wc.EsClient,
		saManager:      wc.Manager,
		metricsHandler: wc.MetricsHandler.WithTags(metrics.OperationTag(metrics.AddSearchAttributesWorkflowScope)),
		logger:         wc.Logger,
	}
}
