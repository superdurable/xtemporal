package replicationadmin

import (
	"context"

	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/service/history/consts"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/replication"
)

func PurgeDLQ(
	ctx context.Context,
	request *historyservice.PurgeDLQMessagesRequest,
	shard historyi.ShardContext,
	replicationDLQHandler replication.DLQHandler,
) (*historyservice.PurgeDLQMessagesResponse, error) {
	_, ok := shard.GetClusterMetadata().GetAllClusterInfo()[request.GetSourceCluster()]
	if !ok {
		return nil, consts.ErrUnknownCluster
	}

	if err := replicationDLQHandler.PurgeMessages(
		ctx,
		request.GetSourceCluster(),
		request.GetInclusiveEndMessageId(),
	); err != nil {
		return nil, err
	}
	return &historyservice.PurgeDLQMessagesResponse{}, nil
}
