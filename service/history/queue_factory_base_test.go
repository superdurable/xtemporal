package history

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/client"
	"github.com/superdurable/temporal-server/common/clock"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/membership"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"github.com/superdurable/temporal-server/common/persistence/visibility/manager"
	"github.com/superdurable/temporal-server/common/resource"
	"github.com/superdurable/temporal-server/common/sdk"
	"github.com/superdurable/temporal-server/common/telemetry"
	"github.com/superdurable/temporal-server/common/worker_versioning"
	"github.com/superdurable/temporal-server/service/history/archival"
	"github.com/superdurable/temporal-server/service/history/configs"
	"github.com/superdurable/temporal-server/service/history/replication/eventhandler"
	"github.com/superdurable/temporal-server/service/history/shard"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"github.com/superdurable/temporal-server/service/history/workflow"
	"github.com/superdurable/temporal-server/service/history/workflow/cache"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/mock/gomock"
)

// TestQueueModule_ArchivalQueue tests that the archival queue is created if and only if the task category exists.
func TestQueueModule_ArchivalQueue(t *testing.T) {
	for _, c := range []moduleTestCase{
		{
			Name:                "Archival disabled",
			CategoryExists:      false,
			ExpectArchivalQueue: false,
		},
		{
			Name:                "Archival enabled",
			CategoryExists:      true,
			ExpectArchivalQueue: true,
		},
	} {
		c := c
		t.Run(c.Name, c.Run)
	}
}

// moduleTestCase is a test case for the QueueModule.
type moduleTestCase struct {
	Name                string
	ExpectArchivalQueue bool
	CategoryExists      bool
}

// Run runs the test case.
func (c *moduleTestCase) Run(t *testing.T) {

	controller := gomock.NewController(t)
	dependencies := getModuleDependencies(controller, c)
	var factories []QueueFactory

	app := fx.New(
		dependencies,
		QueueModule,
		fx.Invoke(func(params QueueFactoriesLifetimeHookParams) {
			factories = params.Factories
		}),
	)

	require.NoError(t, app.Err())
	require.NotNil(t, factories)
	var (
		txq QueueFactory
		tiq QueueFactory
		viq QueueFactory
		aq  QueueFactory
	)
	for _, f := range factories {
		switch f.(type) {
		case *transferQueueFactory:
			require.Nil(t, txq)
			txq = f
		case *timerQueueFactory:
			require.Nil(t, tiq)
			tiq = f
		case *visibilityQueueFactory:
			require.Nil(t, viq)
			viq = f
		case *archivalQueueFactory:
			require.Nil(t, aq)
			aq = f
		}
	}
	require.NotNil(t, txq)
	require.NotNil(t, tiq)
	require.NotNil(t, viq)
	if c.ExpectArchivalQueue {
		require.NotNil(t, aq)
	} else {
		require.Nil(t, aq)
	}
}

// getModuleDependencies returns an fx.Option that provides all the dependencies needed for the queue module.
func getModuleDependencies(controller *gomock.Controller, c *moduleTestCase) fx.Option {
	cfg := configs.NewConfig(
		dynamicconfig.NewNoopCollection(),
		1,
	)
	clusterMetadata := cluster.NewMockMetadata(controller)
	clusterMetadata.EXPECT().GetCurrentClusterName().Return("module-test-cluster-name").AnyTimes()
	lazyLoadedOwnershipBasedQuotaScaler := shard.LazyLoadedOwnershipBasedQuotaScaler{
		Value: &atomic.Value{},
	}
	registry := tasks.NewDefaultTaskCategoryRegistry()
	if c.CategoryExists {
		registry.AddCategory(tasks.CategoryArchival)
	}
	serializer := serialization.NewSerializer()
	historyFetcher := eventhandler.NewMockHistoryPaginatedFetcher(controller)
	return fx.Supply(
		unusedDependencies{},
		cfg,
		fx.Annotate(registry, fx.As(new(tasks.TaskCategoryRegistry))),
		fx.Annotate(metrics.NoopMetricsHandler, fx.As(new(metrics.Handler))),
		fx.Annotate(log.NewTestLogger(), fx.As(new(log.SnTaggedLogger))),
		fx.Annotate(clusterMetadata, fx.As(new(cluster.Metadata))),
		lazyLoadedOwnershipBasedQuotaScaler,
		fx.Annotate(serializer, fx.As(new(serialization.Serializer))),
		fx.Annotate(historyFetcher, fx.As(new(eventhandler.HistoryPaginatedFetcher))),
		fx.Annotate(telemetry.NoopTracerProvider, fx.As(new(trace.TracerProvider))),
	)
}

// This is a struct that provides nil implementations of all the dependencies needed for the queue module that are not
// actually used.
type unusedDependencies struct {
	fx.Out

	clock.TimeSource
	membership.ServiceResolver
	namespace.Registry
	client.Bean
	sdk.ClientFactory
	resource.MatchingRawClient
	resource.HistoryRawClient
	manager.VisibilityManager
	archival.Archiver
	workflow.RelocatableAttributesFetcher
	persistence.HistoryTaskQueueManager
	cache.Cache
	chasm.Engine
	ChasmRegistry *chasm.Registry
	worker_versioning.VersionMembershipCache
}
