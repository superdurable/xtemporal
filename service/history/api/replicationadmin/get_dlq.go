package replicationadmin

import (
	"context"

	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/service/history/consts"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/replication"
)

func GetDLQ(
	ctx context.Context,
	request *historyservice.GetDLQMessagesRequest,
	shard historyi.ShardContext,
	replicationDLQHandler replication.DLQHandler,
) (*historyservice.GetDLQMessagesResponse, error) {
	_, ok := shard.GetClusterMetadata().GetAllClusterInfo()[request.GetSourceCluster()]
	if !ok {
		return nil, consts.ErrUnknownCluster
	}

	tasks, tasksInfo, token, err := replicationDLQHandler.GetMessages(
		ctx,
		request.GetSourceCluster(),
		request.GetInclusiveEndMessageId(),
		int(request.GetMaximumPageSize()),
		request.GetNextPageToken(),
	)
	if err != nil {
		return nil, err
	}
	return &historyservice.GetDLQMessagesResponse{
		Type:                 request.GetType(),
		ReplicationTasks:     tasks,
		ReplicationTasksInfo: tasksInfo,
		NextPageToken:        token,
	}, nil
}
