package replication

import (
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/client"
	"github.com/superdurable/xtemporal/common/cluster"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	ctasks "github.com/superdurable/xtemporal/common/tasks"
	"github.com/superdurable/xtemporal/common/testing/testhooks"
	"github.com/superdurable/xtemporal/service/history/configs"
	"github.com/superdurable/xtemporal/service/history/replication/eventhandler"
	"github.com/superdurable/xtemporal/service/history/shard"
	wcache "github.com/superdurable/xtemporal/service/history/workflow/cache"
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
		ChasmRegistry             *chasm.Registry
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
		TestHooks                testhooks.TestHooks
		DLQWriter                DLQWriter
		HistoryEventsHandler     eventhandler.HistoryEventsHandler
		WorkflowCache            wcache.Cache
		RemoteHistoryFetcher     eventhandler.HistoryPaginatedFetcher
	}
)
