package listtasks

import (
	"context"
	"fmt"

	"go.temporal.io/api/serviceerror"
	"github.com/superdurable/xtemporal/api/adminservice/v1"
	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/common"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/primitives/timestamp"
	"github.com/superdurable/xtemporal/service/history/tasks"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Invoke(
	ctx context.Context,
	taskCategoryRegistry tasks.TaskCategoryRegistry,
	executionManager persistence.ExecutionManager,
	request *historyservice.ListTasksRequest,
) (*historyservice.ListTasksResponse, error) {
	adminRequest := request.Request
	taskCategory, ok := taskCategoryRegistry.GetCategoryByID(int(adminRequest.Category))
	if !ok {
		return nil, &serviceerror.InvalidArgument{
			Message: fmt.Sprintf("unknown task category: %v", adminRequest.Category),
		}
	}

	taskRange := adminRequest.GetTaskRange()
	var minTaskKey, maxTaskKey tasks.Key
	if taskRange.InclusiveMinTaskKey != nil {
		minTaskKey = tasks.NewKey(
			timestamp.TimeValue(taskRange.InclusiveMinTaskKey.FireTime),
			taskRange.InclusiveMinTaskKey.TaskId,
		)
		if err := tasks.ValidateKey(minTaskKey); err != nil {
			return nil, &serviceerror.InvalidArgument{
				Message: fmt.Sprintf("invalid minTaskKey: %v", err.Error()),
			}
		}
	}
	if taskRange.ExclusiveMaxTaskKey != nil {
		maxTaskKey = tasks.NewKey(
			timestamp.TimeValue(taskRange.ExclusiveMaxTaskKey.FireTime),
			taskRange.ExclusiveMaxTaskKey.TaskId,
		)
		if err := tasks.ValidateKey(maxTaskKey); err != nil {
			return nil, &serviceerror.InvalidArgument{
				Message: fmt.Sprintf("invalid maxTaskKey: %v", err.Error()),
			}
		}
	}

	resp, err := executionManager.GetHistoryTasks(ctx, &persistence.GetHistoryTasksRequest{
		ShardID:             adminRequest.ShardId,
		TaskCategory:        taskCategory,
		InclusiveMinTaskKey: minTaskKey,
		ExclusiveMaxTaskKey: maxTaskKey,
		BatchSize:           int(adminRequest.BatchSize),
		NextPageToken:       adminRequest.NextPageToken,
	})
	if err != nil {
		return nil, err
	}

	return &historyservice.ListTasksResponse{
		Response: &adminservice.ListHistoryTasksResponse{
			Tasks:         toAdminTask(resp.Tasks),
			NextPageToken: resp.NextPageToken,
		},
	}, nil
}

func toAdminTask(historyTasks []tasks.Task) []*adminservice.Task {
	var adminTasks []*adminservice.Task
	for _, historyTask := range historyTasks {
		historyTaskVersion := common.EmptyVersion
		if taskWithVersion, ok := historyTask.(tasks.HasVersion); ok {
			historyTaskVersion = taskWithVersion.GetVersion()
		}

		adminTasks = append(adminTasks, &adminservice.Task{
			NamespaceId: historyTask.GetNamespaceID(),
			WorkflowId:  historyTask.GetWorkflowID(),
			RunId:       historyTask.GetRunID(),
			TaskId:      historyTask.GetTaskID(),
			TaskType:    historyTask.GetType(),
			FireTime:    timestamppb.New(historyTask.GetKey().FireTime),
			Version:     historyTaskVersion,
		})
	}
	return adminTasks
}
