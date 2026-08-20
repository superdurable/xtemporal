package replication

import (
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/client"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	ctasks "github.com/superdurable/temporal-server/common/tasks"
	"github.com/superdurable/temporal-server/service/history/configs"
	"github.com/superdurable/temporal-server/service/history/replication/eventhandler"
	"github.com/superdurable/temporal-server/service/history/shard"
	wcache "github.com/superdurable/temporal-server/service/history/workflow/cache"
	"go.uber.org/fx"
)

type (
	ProcessToolBox struct {
		fx.In

		Config                    *configs.Config
		ClusterMetadata           cluster.Metadata
		ClientBean                client.Bean
		ShardController           shard.Controller
		ChasmEngine               chasm.Engine
		NamespaceCache            namespace.Registry
		EagerNamespaceRefresher   EagerNamespaceRefresher
		ResendHandler             eventhandler.ResendHandler
		HighPriorityTaskScheduler ctasks.Scheduler[TrackableExecutableTask] `name:"HighPriorityTaskScheduler"`
		// consider using a single TaskScheduler i.e. InterleavedWeightedRoundRobinScheduler instead of two
		LowPriorityTaskScheduler ctasks.Scheduler[TrackableExecutableTask] `name:"LowPriorityTaskScheduler"`
		MetricsHandler           metrics.Handler
		Logger                   log.Logger
		ThrottledLogger          log.ThrottledLogger
		Serializer               serialization.Serializer
		DLQWriter                DLQWriter
		HistoryEventsHandler     eventhandler.HistoryEventsHandler
		WorkflowCache            wcache.Cache
		RemoteHistoryFetcher     eventhandler.HistoryPaginatedFetcher
	}
)
