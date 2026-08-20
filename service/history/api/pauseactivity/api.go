package pauseactivity

import (
	"context"

	"github.com/superdurable/temporal-server/api/historyservice/v1"
	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/service/history/api"
	"github.com/superdurable/temporal-server/service/history/consts"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/workflow"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Invoke(
	ctx context.Context,
	request *historyservice.PauseActivityRequest,
	shardContext historyi.ShardContext,
	workflowConsistencyChecker api.WorkflowConsistencyChecker,
) (resp *historyservice.PauseActivityResponse, retError error) {

	err := api.GetAndUpdateWorkflowWithNew(
		ctx,
		nil,
		definition.NewWorkflowKey(
			request.NamespaceId,
			request.GetFrontendRequest().GetExecution().GetWorkflowId(),
			request.GetFrontendRequest().GetExecution().GetRunId(),
		),
		func(workflowLease api.WorkflowLease) (*api.UpdateWorkflowAction, error) {
			mutableState := workflowLease.GetMutableState()
			frontendRequest := request.GetFrontendRequest()
			var activityIDs []string
			switch a := frontendRequest.GetActivity().(type) {
			case *workflowservice.PauseActivityRequest_Id:
				activityIDs = append(activityIDs, a.Id)
			case *workflowservice.PauseActivityRequest_Type:
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

			pauseInfo := &persistencespb.ActivityInfo_PauseInfo{
				PauseTime: timestamppb.New(shardContext.GetTimeSource().Now()),
				PausedBy: &persistencespb.ActivityInfo_PauseInfo_Manual_{
					Manual: &persistencespb.ActivityInfo_PauseInfo_Manual{
						Identity: frontendRequest.GetIdentity(),
						Reason:   frontendRequest.GetReason(),
					},
				},
			}

			for _, activityId := range activityIDs {
				err := workflow.PauseActivity(mutableState, activityId, pauseInfo)
				if err != nil {
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
	if _, ok := request.GetFrontendRequest().GetActivity().(*workflowservice.PauseActivityRequest_Id); ok {
		targetingMethod = "id"
	}
	if ns, err := shardContext.GetNamespaceRegistry().GetNamespaceByID(namespace.ID(request.NamespaceId)); err == nil {
		metrics.ActivityPauseRequests.With(shardContext.GetMetricsHandler().WithTags(
			metrics.NamespaceTag(ns.Name().String()),
			metrics.ActivityTargetingMethodTag(targetingMethod),
		)).Record(1)
	}

	return &historyservice.PauseActivityResponse{}, nil
}
