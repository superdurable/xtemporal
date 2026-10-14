package resetactivity

import (
	"context"

	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/common/definition"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/service/history/api"
	"github.com/superdurable/xtemporal/service/history/consts"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/workflow"
	"go.temporal.io/api/workflowservice/v1"
)

func Invoke(
	ctx context.Context,
	req *historyservice.ResetActivityRequest,
	shardContext historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (resp *historyservice.ResetActivityResponse, retError error) {
	request := req.GetFrontendRequest()
	var activityMetrics []workflow.ActivityMetricsInfo

	workflowKey := definition.NewWorkflowKey(
		req.NamespaceId,
		request.GetExecution().GetWorkflowId(),
		request.GetExecution().GetRunId(),
	)

	err := api.GetAndUpdateWorkflowWithNew(
		ctx,
		nil,
		workflowKey,
		func(workflowLease api.WorkflowLease) (*api.UpdateWorkflowAction, error) {
			mutableState := workflowLease.GetMutableState()
			if !mutableState.IsWorkflowExecutionRunning() {
				return nil, consts.ErrWorkflowCompleted
			}

			var activityIDs []string
			switch a := request.GetActivity().(type) {
			case *workflowservice.ResetActivityRequest_Id:
				activityIDs = append(activityIDs, a.Id)
			case *workflowservice.ResetActivityRequest_Type:
				activityType := a.Type
				for _, ai := range mutableState.GetPendingActivityInfos() {
					if ai.ActivityType.Name == activityType {
						activityIDs = append(activityIDs, ai.ActivityId)
					}
				}
			case *workflowservice.ResetActivityRequest_MatchAll:
				for _, ai := range mutableState.GetPendingActivityInfos() {
					activityIDs = append(activityIDs, ai.ActivityId)
				}
			}

			if len(activityIDs) == 0 {
				return nil, consts.ErrActivityNotFound
			}

			for _, activityId := range activityIDs {
				activityInfo, activityFound := mutableState.GetActivityByActivityID(activityId)
				if !activityFound {
					return nil, consts.ErrActivityNotFound
				}
				if err := workflow.ResetActivity(
					ctx,
					shardContext, mutableState, activityId,
					request.ResetHeartbeat, request.KeepPaused, request.RestoreOriginalOptions,
					request.Jitter.AsDuration(),
				); err != nil {
					return nil, err
				}
				activityMetrics = append(activityMetrics, workflow.NewActivityMetricsInfo(mutableState, activityInfo))
			}
			return &api.UpdateWorkflowAction{
				Noop:               false,
				CreateWorkflowTask: false,
			}, nil
		},
		nil,
		shardContext,
		workflowConsistencyChecker,
	)

	if err != nil {
		return nil, err
	}

	for _, info := range activityMetrics {
		metrics.ActivityReset.With(info.MetricsHandler(shardContext, metrics.ActivityResetScope)).Record(1)
	}

	return &historyservice.ResetActivityResponse{}, nil
}
