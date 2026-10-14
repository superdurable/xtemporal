package history

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	persistencespb "github.com/superdurable/xtemporal/api/persistence/v1"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/client"
	"github.com/superdurable/xtemporal/common/circuitbreaker"
	"github.com/superdurable/xtemporal/common/clock"
	"github.com/superdurable/xtemporal/common/definition"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	"github.com/superdurable/xtemporal/common/telemetry"
	"github.com/superdurable/xtemporal/service/history/circuitbreakerpool"
	"github.com/superdurable/xtemporal/service/history/configs"
	"github.com/superdurable/xtemporal/service/history/queues"
	"github.com/superdurable/xtemporal/service/history/replication/eventhandler"
	"github.com/superdurable/xtemporal/service/history/shard"
	"github.com/superdurable/xtemporal/service/history/tasks"
	"github.com/superdurable/xtemporal/service/history/tests"
	"github.com/superdurable/xtemporal/service/history/workflow/cache"
	"go.uber.org/mock/gomock"
)

func TestOutboundQueueFactory_ChasmTaskGroupWiring(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	chasmRegistry := chasm.NewRegistry(log.NewTestLogger())
	lib := chasm.NewMockLibrary(ctrl)
	lib.EXPECT().Name().Return("TestLib").AnyTimes()
	lib.EXPECT().Components().Return(nil)
	lib.EXPECT().NexusServices().Return(nil)
	lib.EXPECT().NexusServiceProcessors().Return(nil)
	type testTaskType struct{}
	lib.EXPECT().Tasks().Return([]*chasm.RegistrableTask{
		chasm.NewRegistrableSideEffectTask(
			"MyTask",
			chasm.NewMockSideEffectTaskHandler[*chasm.MockComponent, testTaskType](ctrl),
			chasm.WithTaskGroup("my-task-group"),
		),
	})
	require.NoError(t, chasmRegistry.Register(lib))

	config := tests.NewDynamicConfig()
	mockShard := shard.NewTestContext(ctrl, &persistencespb.ShardInfo{
		ShardId: 1,
		RangeId: 1,
		Owner:   "test-owner",
	}, config)
	mockShard.SetChasmRegistry(chasmRegistry)
	mockShard.Resource.ClusterMetadata.EXPECT().GetCurrentClusterName().Return("active").AnyTimes()
	mockShard.Resource.ClusterMetadata.EXPECT().GetClusterID().Return(int64(1)).AnyTimes()
	mockShard.Resource.NamespaceCache.EXPECT().GetNamespaceByID(gomock.Any()).Return(tests.GlobalNamespaceEntry, nil).AnyTimes()
	mockShard.Resource.NamespaceCache.EXPECT().GetNamespaceName(gomock.Any()).Return(tests.Namespace, nil).AnyTimes()

	taskTypeID := chasm.GenerateTypeID("TestLib.MyTask")
	chasmTask := &tasks.ChasmTask{
		WorkflowKey: definition.NewWorkflowKey(tests.NamespaceID.String(), "wf-id", "run-id"),
		Category:    tasks.CategoryOutbound,
		Destination: "test-destination",
		TaskID:      1,
		Info:        &persistencespb.ChasmTaskInfo{TypeId: taskTypeID},
	}
	require.Empty(t, chasmTask.OutboundTaskGroup(), "task group should not be set before loading through queue")

	mockShard.Resource.ExecutionMgr.EXPECT().GetHistoryTasks(gomock.Any(), gomock.Any()).Return(
		&persistence.GetHistoryTasksResponse{
			Tasks: []tasks.Task{chasmTask},
		}, nil,
	).AnyTimes()

	nsRegistry := namespace.NewMockRegistry(ctrl)
	nsRegistry.EXPECT().GetNamespaceName(gomock.Any()).Return(namespace.Name("test-ns"), nil).AnyTimes()

	rateLimiter, err := queues.NewPrioritySchedulerRateLimiter(
		func(string) float64 { return 100 },
		func() float64 { return 100 },
		func(string) float64 { return 100 },
		func() float64 { return 100 },
	)
	require.NoError(t, err)

	cbPool := &circuitbreakerpool.OutboundQueueCircuitBreakerPool{
		CircuitBreakerPool: circuitbreakerpool.NewCircuitBreakerPool(
			func(tasks.TaskGroupNamespaceIDAndDestination) circuitbreaker.TwoStepCircuitBreaker {
				cb := circuitbreaker.NewTwoStepCircuitBreakerWithDynamicSettings(circuitbreaker.Settings{Name: "test"})
				cb.UpdateSettings(dynamicconfig.CircuitBreakerSettings{})
				return cb
			},
		),
	}

	taskCh := make(chan tasks.Task, 1)
	factory := NewOutboundQueueFactory(outboundQueueFactoryParams{
		QueueFactoryBaseParams: QueueFactoryBaseParams{
			NamespaceRegistry:    nsRegistry,
			ClusterMetadata:      mockShard.Resource.ClusterMetadata,
			WorkflowCache:        cache.NewMockCache(ctrl),
			Config:               configs.NewConfig(dynamicconfig.NewNoopCollection(), 1),
			TimeSource:           clock.NewRealTimeSource(),
			MetricsHandler:       metrics.NoopMetricsHandler,
			TracerProvider:       telemetry.NoopTracerProvider,
			Logger:               log.NewTestLogger(),
			SchedulerRateLimiter: rateLimiter,
			DLQWriter:            nil,
			ExecutorWrapper:      &captureExecutorWrapper{taskCh: taskCh},
			Serializer:           serialization.NewSerializer(),
			RemoteHistoryFetcher: eventhandler.NewMockHistoryPaginatedFetcher(ctrl),
			ChasmEngine:          chasm.NewMockEngine(ctrl),
			ChasmRegistry:        chasmRegistry,
		},
		ClientBean:         client.NewMockBean(ctrl),
		CircuitBreakerPool: cbPool,
		MatchingClient:     nil,
	})

	queue := factory.CreateQueue(mockShard)
	require.NotNil(t, queue)

	factory.Start()
	defer factory.Stop()

	queue.Start()
	defer queue.Stop()

	queue.NotifyNewTasks([]tasks.Task{chasmTask})

	select {
	case executedTask := <-taskCh:
		ct, ok := executedTask.(*tasks.ChasmTask)
		require.True(t, ok, "expected ChasmTask, got %T", executedTask)
		require.Equal(t, "my-task-group", ct.OutboundTaskGroup())
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for task to reach executor")
	}
}

// captureExecutorWrapper intercepts tasks at the executor level.
type captureExecutorWrapper struct {
	taskCh chan<- tasks.Task
}

func (w *captureExecutorWrapper) Wrap(delegate queues.Executor) queues.Executor {
	return &captureExecutor{taskCh: w.taskCh}
}

// captureExecutor captures the first task it sees and sends it to the channel.
type captureExecutor struct {
	taskCh chan<- tasks.Task
}

func (e *captureExecutor) Execute(_ context.Context, executable queues.Executable) queues.ExecuteResponse {
	select {
	case e.taskCh <- executable.GetTask():
	default:
	}
	return queues.ExecuteResponse{}
}
