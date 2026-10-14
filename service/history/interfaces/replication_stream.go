package interfaces

import (
	"context"
	"time"

	replicationspb "github.com/superdurable/xtemporal/api/replication/v1"
	"github.com/superdurable/xtemporal/common/collection"
	"github.com/superdurable/xtemporal/service/history/tasks"
)

type (
	ReplicationStream interface {
		SubscribeReplicationNotification(string) (<-chan struct{}, string)
		UnsubscribeReplicationNotification(string)
		ConvertReplicationTask(
			ctx context.Context,
			task tasks.Task,
			clusterID int32,
		) (*replicationspb.ReplicationTask, error)

		GetReplicationTasksIter(
			ctx context.Context,
			pollingCluster string,
			minInclusiveTaskID int64,
			maxExclusiveTaskID int64,
		) (collection.Iterator[tasks.Task], error)

		GetMaxReplicationTaskInfo() (int64, time.Time)
	}
)
