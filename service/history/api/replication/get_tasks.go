package replication

import (
	"context"
	"fmt"
	"math"
	"time"

	enumsspb "github.com/superdurable/temporal-server/api/enums/v1"
	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	replicationspb "github.com/superdurable/temporal-server/api/replication/v1"
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/common/persistence"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/replication"
	"github.com/superdurable/temporal-server/service/history/shard"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"go.temporal.io/api/serviceerror"
)

func GetTasks(
	ctx context.Context,
	shardContext historyi.ShardContext,
	replicationAckMgr replication.AckManager,
	pollingCluster string,
	ackMessageID int64,
	ackTimestamp time.Time,
	queryMessageID int64,
) (*replicationspb.ReplicationMessages, error) {
	allClusterInfo := shardContext.GetClusterMetadata().GetAllClusterInfo()
	clusterInfo, ok := allClusterInfo[pollingCluster]
	if !ok {
		return nil, serviceerror.NewInternal(
			fmt.Sprintf("missing cluster info for cluster: %v", pollingCluster),
		)
	}
	readerID := shard.ReplicationReaderIDFromClusterShardID(
		clusterInfo.InitialFailoverVersion,
		shardContext.GetShardID(),
	)

	if ackMessageID != persistence.EmptyQueueMessageID {
		if err := shardContext.UpdateReplicationQueueReaderState(
			readerID,
			&persistencespb.QueueReaderState{
				Scopes: []*persistencespb.QueueSliceScope{{
					Range: &persistencespb.QueueSliceRange{
						InclusiveMin: shard.ConvertToPersistenceTaskKey(
							tasks.NewImmediateKey(ackMessageID + 1),
						),
						ExclusiveMax: shard.ConvertToPersistenceTaskKey(
							tasks.NewImmediateKey(math.MaxInt64),
						),
					},
					Predicate: &persistencespb.Predicate{
						PredicateType: enumsspb.PREDICATE_TYPE_UNIVERSAL,
						Attributes:    &persistencespb.Predicate_UniversalPredicateAttributes{},
					},
				}},
			},
		); err != nil {
			shardContext.GetLogger().Error("error updating replication level for shard", tag.Error(err), tag.OperationFailed)
		}
		shardContext.UpdateRemoteClusterInfo(pollingCluster, ackMessageID, ackTimestamp)
	}

	replicationMessages, err := replicationAckMgr.GetTasks(
		ctx,
		pollingCluster,
		queryMessageID,
	)
	if err != nil {
		shardContext.GetLogger().Error("Failed to retrieve replication messages.", tag.Error(err))
		return nil, err
	}

	shardContext.GetLogger().Debug("Successfully fetched replication messages.", tag.Counter(len(replicationMessages.ReplicationTasks)))
	return replicationMessages, nil
}
