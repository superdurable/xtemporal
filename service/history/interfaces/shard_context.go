package interfaces

import (
	"context"
	"time"

	"github.com/superdurable/temporal-server/api/adminservice/v1"
	clockspb "github.com/superdurable/temporal-server/api/clock/v1"
	"github.com/superdurable/temporal-server/api/historyservice/v1"
	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/archiver"
	"github.com/superdurable/temporal-server/common/clock"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/finalizer"
	"github.com/superdurable/temporal-server/common/locks"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"github.com/superdurable/temporal-server/common/pingable"
	"github.com/superdurable/temporal-server/common/searchattribute"
	"github.com/superdurable/temporal-server/service/history/configs"
	"github.com/superdurable/temporal-server/service/history/events"
	"github.com/superdurable/temporal-server/service/history/hsm"
	"github.com/superdurable/temporal-server/service/history/tasks"
	commonpb "go.temporal.io/api/common/v1"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination shard_context_mock.go

type (
	// ShardContext represents a history engine shard
	ShardContext interface {
		GetShardID() int32
		GetRangeID() int64
		GetOwner() string
		GetExecutionManager() persistence.ExecutionManager
		GetNamespaceRegistry() namespace.Registry
		GetClusterMetadata() cluster.Metadata
		GetConfig() *configs.Config
		GetEventsCache() events.Cache
		GetLogger() log.Logger
		GetThrottledLogger() log.Logger
		GetMetricsHandler() metrics.Handler
		GetTimeSource() clock.TimeSource

		GetRemoteAdminClient(string) (adminservice.AdminServiceClient, error)
		GetHistoryClient() historyservice.HistoryServiceClient
		GetPayloadSerializer() serialization.Serializer

		GetSearchAttributesProvider() searchattribute.Provider
		GetSearchAttributesMapperProvider() searchattribute.MapperProvider
		GetArchivalMetadata() archiver.ArchivalMetadata

		GetEngine(ctx context.Context) (Engine, error)

		AssertOwnership(ctx context.Context) error
		NewVectorClock() (*clockspb.VectorClock, error)
		CurrentVectorClock() *clockspb.VectorClock

		GenerateTaskID() (int64, error)
		GenerateTaskIDs(number int) ([]int64, error)

		GetQueueExclusiveHighReadWatermark(category tasks.Category) tasks.Key
		GetQueueState(category tasks.Category) (*persistencespb.QueueState, bool)
		SetQueueState(category tasks.Category, tasksCompleted int, state *persistencespb.QueueState) error
		UpdateReplicationQueueReaderState(readerID int64, readerState *persistencespb.QueueReaderState) error

		GetReplicatorDLQAckLevel(sourceCluster string) int64
		UpdateReplicatorDLQAckLevel(sourCluster string, ackLevel int64) error

		UpdateRemoteClusterInfo(cluster string, ackTaskID int64, ackTimestamp time.Time)
		UpdateRemoteReaderInfo(readerID int64, ackTaskID int64, ackTimestamp time.Time) error

		SetCurrentTime(cluster string, currentTime time.Time)
		GetCurrentTime(cluster string) time.Time

		GetReplicationStatus(cluster []string) (map[string]*historyservice.ShardReplicationStatusPerCluster, map[string]*historyservice.HandoverNamespaceInfo, error)

		UpdateHandoverNamespace(ns *namespace.Namespace, deletedFromDb bool)

		AppendHistoryEvents(ctx context.Context, request *persistence.AppendHistoryNodesRequest, namespaceID namespace.ID, execution *commonpb.WorkflowExecution) (int, error)

		AddTasks(ctx context.Context, request *persistence.AddHistoryTasksRequest) error
		AddSpeculativeWorkflowTaskTimeoutTask(task *tasks.WorkflowTaskTimeoutTask) error
		GetHistoryTasks(ctx context.Context, request *persistence.GetHistoryTasksRequest) (*persistence.GetHistoryTasksResponse, error)
		CreateWorkflowExecution(ctx context.Context, request *persistence.CreateWorkflowExecutionRequest) (*persistence.CreateWorkflowExecutionResponse, error)
		UpdateWorkflowExecution(ctx context.Context, request *persistence.UpdateWorkflowExecutionRequest) (*persistence.UpdateWorkflowExecutionResponse, error)
		ConflictResolveWorkflowExecution(ctx context.Context, request *persistence.ConflictResolveWorkflowExecutionRequest) (*persistence.ConflictResolveWorkflowExecutionResponse, error)
		SetWorkflowExecution(ctx context.Context, request *persistence.SetWorkflowExecutionRequest) (*persistence.SetWorkflowExecutionResponse, error)
		GetCurrentExecution(ctx context.Context, request *persistence.GetCurrentExecutionRequest) (*persistence.GetCurrentExecutionResponse, error)
		GetWorkflowExecution(ctx context.Context, request *persistence.GetWorkflowExecutionRequest) (*persistence.GetWorkflowExecutionResponse, error)
		// DeleteWorkflowExecution add task to delete visibility, current workflow execution, and deletes workflow execution.
		// If branchToken != nil, then delete history also, otherwise leave history.
		DeleteWorkflowExecution(ctx context.Context, workflowKey definition.WorkflowKey, archetypeID chasm.ArchetypeID, branchToken []byte, closeExecutionVisibilityTaskID int64, workflowCloseTime time.Time, stage *tasks.DeleteWorkflowExecutionStage) error

		GetCachedWorkflowContext(ctx context.Context, namespaceID namespace.ID, execution *commonpb.WorkflowExecution, lockPriority locks.Priority) (WorkflowContext, ReleaseWorkflowContextFunc, error)
		GetCurrentCachedWorkflowContext(ctx context.Context, namespaceID namespace.ID, workflowID string, lockPriority locks.Priority) (ReleaseWorkflowContextFunc, error)

		UnloadForOwnershipLost()

		StateMachineRegistry() *hsm.Registry
		GetFinalizer() *finalizer.Finalizer

		ChasmRegistry() *chasm.Registry
	}

	// A ControllableContext is a Context plus other methods needed by
	// the Controller.
	ControllableContext interface {
		ShardContext
		pingable.Pingable

		IsValid() bool
		FinishStop()
	}
)
