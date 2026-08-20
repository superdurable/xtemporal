package replicationadmin

import (
	"context"

	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/service/history/consts"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/replication"
)

func MergeDLQ(
	ctx context.Context,
	request *historyservice.MergeDLQMessagesRequest,
	shard historyi.ShardContext,
	replicationDLQHandler replication.DLQHandler,
) (*historyservice.MergeDLQMessagesResponse, error) {
	_, ok := shard.GetClusterMetadata().GetAllClusterInfo()[request.GetSourceCluster()]
	if !ok {
		return nil, consts.ErrUnknownCluster
	}

	token, err := replicationDLQHandler.MergeMessages(
		ctx,
		request.GetSourceCluster(),
		request.GetInclusiveEndMessageId(),
		int(request.GetMaximumPageSize()),
		request.GetNextPageToken(),
	)
	if err != nil {
		return nil, err
	}
	return &historyservice.MergeDLQMessagesResponse{
		NextPageToken: token,
	}, nil
}
