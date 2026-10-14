package deleteexecution

import (
	"context"

	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/chasm"
)

func Invoke(
	ctx context.Context,
	chasmEngine chasm.Engine,
	request *historyservice.DeleteExecutionRequest,
) (*historyservice.DeleteExecutionResponse, error) {
	key := chasm.ExecutionKey{
		NamespaceID: request.GetNamespaceId(),
		BusinessID:  request.GetExecution().GetWorkflowId(),
		RunID:       request.GetExecution().GetRunId(),
	}
	ref := chasm.NewComponentRefByArchetypeID(key, request.GetArchetypeId())
	if err := chasmEngine.DeleteExecution(ctx, ref, chasm.DeleteExecutionRequest{
		TerminateComponentRequest: chasm.TerminateComponentRequest{
			Reason:   request.GetReason(),
			Identity: request.GetIdentity(),
		},
	}); err != nil {
		return nil, err
	}
	return &historyservice.DeleteExecutionResponse{}, nil
}
