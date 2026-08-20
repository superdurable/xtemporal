package history

import (
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/client"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"github.com/superdurable/temporal-server/common/persistence/visibility/manager"
	"github.com/superdurable/temporal-server/common/resource"
	"github.com/superdurable/temporal-server/common/sdk"
	"github.com/superdurable/temporal-server/common/testing/testhooks"
	"github.com/superdurable/temporal-server/common/worker_versioning"
	"github.com/superdurable/temporal-server/service/history/api"
	"github.com/superdurable/temporal-server/service/history/circuitbreakerpool"
	"github.com/superdurable/temporal-server/service/history/configs"
	"github.com/superdurable/temporal-server/service/history/events"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/replication"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"github.com/superdurable/temporal-server/service/history/workflow"
	wcache "github.com/superdurable/temporal-server/service/history/workflow/cache"
	"github.com/superdurable/temporal-server/service/worker/workerdeployment"
	"go.opentelemetry.io/otel/trace"
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
		OutboundQueueCBPool             *circuitbreakerpool.OutboundQueueCircuitBreakerPool
		PersistenceRateLimiter          replication.PersistenceRateLimiter
		TestHooks                       testhooks.TestHooks
		ChasmEngine                     chasm.Engine
		VersionMembershipCache          worker_versioning.VersionMembershipCache
		ReactivationSignalCache         worker_versioning.ReactivationSignalCache
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
		f.ReactivationSignalCache,
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
		f.OutboundQueueCBPool,
		f.PersistenceRateLimiter,
		f.TestHooks,
		f.ChasmEngine,
	)
}
