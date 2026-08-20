package resetactivity

import (
	"context"

	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/service/history/api"
	"github.com/superdurable/temporal-server/service/history/consts"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/workflow"
	"go.temporal.io/api/workflowservice/v1"
)

func Invoke(
	ctx context.Context,
	req *historyservice.ResetActivityRequest,
	shardContext historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (resp *historyservice.ResetActivityResponse, retError error) {
	request := req.GetFrontendRequest()

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
			}

			if len(activityIDs) == 0 {
				return nil, consts.ErrActivityNotFound
			}

			for _, activityId := range activityIDs {
				if err := workflow.ResetActivity(
					ctx,
					shardContext, mutableState, activityId,
					request.ResetHeartbeat, request.KeepPaused, request.RestoreOriginalOptions,
					request.Jitter.AsDuration(),
				); err != nil {
					return nil, err
				}
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

	targetingMethod := "type"
	if _, ok := req.GetFrontendRequest().GetActivity().(*workflowservice.ResetActivityRequest_Id); ok {
		targetingMethod = "id"
	}
	if ns, err := shardContext.GetNamespaceRegistry().GetNamespaceByID(namespace.ID(req.NamespaceId)); err == nil {
		metrics.ActivityResetRequests.With(shardContext.GetMetricsHandler().WithTags(
			metrics.NamespaceTag(ns.Name().String()),
			metrics.ActivityTargetingMethodTag(targetingMethod),
		)).Record(1)
	}

	return &historyservice.ResetActivityResponse{}, nil
}
