package shard

import (
	otellog "go.opentelemetry.io/otel/log"
	"github.com/superdurable/xtemporal/chasm"
	chasmworkflow "github.com/superdurable/xtemporal/chasm/lib/workflow"
	"github.com/superdurable/xtemporal/client"
	"github.com/superdurable/xtemporal/common/archiver"
	"github.com/superdurable/xtemporal/common/clock"
	"github.com/superdurable/xtemporal/common/cluster"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/membership"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	commonnexus "github.com/superdurable/xtemporal/common/nexus"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	"github.com/superdurable/xtemporal/common/resource"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/service/history/configs"
	"github.com/superdurable/xtemporal/service/history/events"
	"github.com/superdurable/xtemporal/service/history/hsm"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/tasks"
	"go.uber.org/fx"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination context_factory_mock.go

type (
	CloseCallback func(historyi.ControllableContext)

	ContextFactory interface {
		CreateContext(shardID int32, closeCallback CloseCallback) (historyi.ControllableContext, error)
	}

	ContextFactoryParams struct {
		fx.In

		ArchivalMetadata            archiver.ArchivalMetadata
		ClientBean                  client.Bean
		ClusterMetadata             cluster.Metadata
		Config                      *configs.Config
		PersistenceConfig           config.Persistence
		EngineFactory               EngineFactory
		HistoryClient               resource.HistoryClient
		HistoryServiceResolver      membership.ServiceResolver
		HostInfoProvider            membership.HostInfoProvider
		Logger                      log.Logger
		MetricsHandler              metrics.Handler
		EventLogger                 otellog.Logger
		NamespaceRegistry           namespace.Registry
		PayloadSerializer           serialization.Serializer
		PersistenceExecutionManager persistence.ExecutionManager
		PersistenceShardManager     persistence.ShardManager
		SaMapperProvider            searchattribute.MapperProvider
		SaProvider                  searchattribute.Provider
		ThrottledLogger             log.ThrottledLogger
		TimeSource                  clock.TimeSource
		TaskCategoryRegistry        tasks.TaskCategoryRegistry
		EventsCache                 events.Cache

		StateMachineRegistry   *hsm.Registry
		ChasmRegistry          *chasm.Registry
		ChasmWorkflowRegistry  *chasmworkflow.Registry
		EndpointRegistry       commonnexus.EndpointRegistry
		HandoverTrackerFactory HandoverTrackerFactory
	}

	contextFactoryImpl struct {
		*ContextFactoryParams
	}
)

func ContextFactoryProvider(params ContextFactoryParams) ContextFactory {
	return &contextFactoryImpl{
		ContextFactoryParams: &params,
	}
}

func (c *contextFactoryImpl) CreateContext(
	shardID int32,
	closeCallback CloseCallback,
) (historyi.ControllableContext, error) {
	shard, err := newContext(
		shardID,
		c.EngineFactory,
		c.Config,
		c.PersistenceConfig,
		closeCallback,
		c.Logger,
		c.ThrottledLogger,
		c.PersistenceExecutionManager,
		c.PersistenceShardManager,
		c.ClientBean,
		c.HistoryClient,
		c.MetricsHandler,
		c.EventLogger,
		c.PayloadSerializer,
		c.TimeSource,
		c.NamespaceRegistry,
		c.SaProvider,
		c.SaMapperProvider,
		c.ClusterMetadata,
		c.ArchivalMetadata,
		c.HostInfoProvider,
		c.TaskCategoryRegistry,
		c.EventsCache,
		c.StateMachineRegistry,
		c.ChasmRegistry,
		c.ChasmWorkflowRegistry,
		c.EndpointRegistry,
		c.HandoverTrackerFactory,
	)
	if err != nil {
		return nil, err
	}
	shard.start()
	return shard, nil
}
