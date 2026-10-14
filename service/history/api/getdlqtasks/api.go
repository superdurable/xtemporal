// Package getdlqtasks contains the logic to implement the [historyservice.HistoryServiceServer.GetDLQTasks] API.
package getdlqtasks

import (
	"context"
	"errors"

	"go.temporal.io/api/serviceerror"
	commonspb "github.com/superdurable/xtemporal/api/common/v1"
	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/service/history/api"
	"github.com/superdurable/xtemporal/service/history/consts"
	"github.com/superdurable/xtemporal/service/history/tasks"
)

// Invoke the GetDLQTasks API. All errors returned from this function are already translated into the appropriate type
// from the [serviceerror] package.
func Invoke(
	ctx context.Context,
	historyTaskQueueManager persistence.HistoryTaskQueueManager,
	taskCategoryRegistry tasks.TaskCategoryRegistry,
	req *historyservice.GetDLQTasksRequest,
) (*historyservice.GetDLQTasksResponse, error) {
	category, err := api.GetTaskCategory(int(req.DlqKey.TaskCategory), taskCategoryRegistry)
	if err != nil {
		return nil, err
	}

	response, err := historyTaskQueueManager.ReadRawTasks(ctx, &persistence.ReadTasksRequest{
		QueueKey: persistence.QueueKey{
			QueueType:     persistence.QueueTypeHistoryDLQ,
			Category:      category,
			SourceCluster: req.DlqKey.SourceCluster,
			TargetCluster: req.DlqKey.TargetCluster,
		},
		PageSize:      int(req.PageSize),
		NextPageToken: req.NextPageToken,
	})
	if err != nil {
		if errors.Is(err, persistence.ErrReadTasksNonPositivePageSize) {
			return nil, consts.ErrInvalidPageSize
		}

		return nil, serviceerror.NewUnavailablef("GetDLQTasks failed. Error: %v", err)
	}

	dlqTasks := make([]*commonspb.HistoryDLQTask, len(response.Tasks))
	for i, task := range response.Tasks {
		dlqTasks[i] = &commonspb.HistoryDLQTask{
			Metadata: &commonspb.HistoryDLQTaskMetadata{
				MessageId: task.MessageMetadata.ID,
			},
			Payload: &commonspb.HistoryTask{
				ShardId: task.Payload.ShardId,
				Blob:    task.Payload.Blob,
			},
		}
	}

	return &historyservice.GetDLQTasksResponse{
		DlqTasks:      dlqTasks,
		NextPageToken: response.NextPageToken,
	}, nil
}
