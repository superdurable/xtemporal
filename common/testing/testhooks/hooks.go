package testhooks

import (
	"context"
	"time"

	"github.com/superdurable/xtemporal/api/historyservice/v1"
	persistencespb "github.com/superdurable/xtemporal/api/persistence/v1"
	replicationspb "github.com/superdurable/xtemporal/api/replication/v1"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/namespace"
	historytasks "github.com/superdurable/xtemporal/service/history/tasks"
)

// Test hook keys with their return type and scope.
// Try to avoid global scope as it requires a dedicated test cluster.
var (
	MatchingDisableSyncMatch                 = newKey[bool, namespace.ID]()
	MatchingLBForceReadPartition             = newKey[int, namespace.ID]()
	MatchingLBForceWritePartition            = newKey[int, namespace.ID]()
	UpdateWithStartInBetweenLockAndStart     = newKey[func(), namespace.ID]()
	UpdateWithStartOnClosingWorkflowRetry    = newKey[func(), namespace.ID]()
	TaskQueuesInDeploymentSyncBatchSize      = newKey[int, global]()
	MatchingIgnoreRoutingConfigRevisionCheck = newKey[bool, namespace.ID]()
	MatchingDeploymentRegisterErrorBackoff   = newKey[time.Duration, namespace.ID]()
	MatchingForwardTaskDelay                 = newKey[time.Duration, namespace.ID]()
	HistoryReplicationTaskInterceptor        = newKey[func(*replicationspb.ReplicationTask, func() error) error, global]()
	HistoryReplicationDLQWriteInterceptor    = newKey[func(*persistencespb.ReplicationTaskInfo, func() error) error, global]()
	HistoryChasmRuntimeProvider              = newKey[func(chasm.Engine, chasm.VisibilityManager, *chasm.Registry), global]()
	HistoryTransferTaskInterceptor           = newKey[func(historytasks.Task, func()), namespace.ID]()
	HistoryDLQTaskDeleteInterceptor          = newKey[func(context.Context, *historyservice.DeleteDLQTasksRequest, func(context.Context, *historyservice.DeleteDLQTasksRequest) (*historyservice.DeleteDLQTasksResponse, error)) (*historyservice.DeleteDLQTasksResponse, error), global]()
	NamespaceReplicationTaskInterceptor      = newKey[func(context.Context, *replicationspb.NamespaceTaskAttributes, func() error) error, namespace.Name]()
)

// keyID is a unique identifier for a key, used as a map key.
type keyID = int64

// global is the scope type for global hooks.
type global struct{}

// GlobalScope is the singleton value for global hooks.
var GlobalScope = global{}

// ScopeType indicates the scope of a hook at runtime.
type ScopeType int

const (
	ScopeNamespace ScopeType = iota
	ScopeGlobal
)

type Key[T any, S any] struct {
	id        keyID
	scopeType ScopeType
}
