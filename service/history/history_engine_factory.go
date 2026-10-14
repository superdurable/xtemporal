package history

import (
	"go.opentelemetry.io/otel/trace"
	"github.com/superdurable/xtemporal/chasm"
	chasmworkflow "github.com/superdurable/xtemporal/chasm/lib/workflow"
	"github.com/superdurable/xtemporal/client"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/common/resource"
	"github.com/superdurable/xtemporal/common/sdk"
	"github.com/superdurable/xtemporal/common/testing/testhooks"
	"github.com/superdurable/xtemporal/common/worker_versioning"
	"github.com/superdurable/xtemporal/service/history/api"
	"github.com/superdurable/xtemporal/service/history/circuitbreakerpool"
	"github.com/superdurable/xtemporal/service/history/configs"
	"github.com/superdurable/xtemporal/service/history/events"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/replication"
	"github.com/superdurable/xtemporal/service/history/tasks"
	"github.com/superdurable/xtemporal/service/history/workflow"
	wcache "github.com/superdurable/xtemporal/service/history/workflow/cache"
	"github.com/superdurable/xtemporal/service/worker/workerdeployment"
	"go.uber.org/fx"
)

type (
	HistoryEngineFactoryParams struct {
		fx.In

		ClientBean                      client.Bean
		MatchingClient                  resource.MatchingClient
		SdkClientFactory                sdk.ClientFactory
		EventNotifier                   events.Notifier
		Config                          *configs.Config
		RawMatchingClient               resource.MatchingRawClient
		WorkflowCache                   wcache.Cache
		ReplicationProgressCache        replication.ProgressCache
		Serializer                      serialization.Serializer
		QueueFactories                  []QueueFactory `group:"queueFactory"`
		ReplicationTaskFetcherFactory   replication.TaskFetcherFactory
		ReplicationTaskExecutorProvider replication.TaskExecutorProvider
		TracerProvider                  trace.TracerProvider
		PersistenceVisibilityMgr        manager.VisibilityManager
		EventBlobCache                  persistence.XDCCache
		TaskCategoryRegistry            tasks.TaskCategoryRegistry
		ReplicationDLQWriter            replication.DLQWriter
		CommandHandlerRegistry          *workflow.CommandHandlerRegistry
		ChasmWorkflowRegistry           *chasmworkflow.Registry
		OutboundQueueCBPool             *circuitbreakerpool.OutboundQueueCircuitBreakerPool
		PersistenceRateLimiter          replication.PersistenceRateLimiter
		TestHooks                       testhooks.TestHooks
		ChasmEngine                     chasm.Engine
		VersionMembershipCache          worker_versioning.VersionMembershipAndReactivationStatusCache
		WorkerDeploymentClient          workerdeployment.Client
		RoutingInfoCache                worker_versioning.RoutingInfoCache
	}

	historyEngineFactory struct {
		HistoryEngineFactoryParams
	}
)

func (f *historyEngineFactory) CreateEngine(
	shard historyi.ShardContext,
) historyi.Engine {
	return NewEngineWithShardContext(
		shard,
		f.ClientBean,
		f.MatchingClient,
		f.SdkClientFactory,
		f.EventNotifier,
		f.Config,
		f.VersionMembershipCache,
		f.WorkerDeploymentClient,
		f.RoutingInfoCache,
		f.RawMatchingClient,
		f.WorkflowCache,
		f.ReplicationProgressCache,
		f.Serializer,
		f.QueueFactories,
		f.ReplicationTaskFetcherFactory,
		f.ReplicationTaskExecutorProvider,
		api.NewWorkflowConsistencyChecker(shard, f.WorkflowCache),
		f.TracerProvider,
		f.PersistenceVisibilityMgr,
		f.EventBlobCache,
		f.TaskCategoryRegistry,
		f.ReplicationDLQWriter,
		f.CommandHandlerRegistry,
		f.ChasmWorkflowRegistry,
		f.OutboundQueueCBPool,
		f.PersistenceRateLimiter,
		f.TestHooks,
		f.ChasmEngine,
	)
}
