package frontend

import (
	"context"

	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (wh *WorkflowHandler) CountNexusOperationExecutions(
	context.Context,
	*workflowservice.CountNexusOperationExecutionsRequest,
) (*workflowservice.CountNexusOperationExecutionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CountNexusOperationExecutions is not supported")
}

func (wh *WorkflowHandler) CountWorkers(
	context.Context,
	*workflowservice.CountWorkersRequest,
) (*workflowservice.CountWorkersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CountWorkers is not supported")
}

func (wh *WorkflowHandler) DeleteNexusOperationExecution(
	context.Context,
	*workflowservice.DeleteNexusOperationExecutionRequest,
) (*workflowservice.DeleteNexusOperationExecutionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DeleteNexusOperationExecution is not supported")
}

func (wh *WorkflowHandler) DescribeNexusOperationExecution(
	context.Context,
	*workflowservice.DescribeNexusOperationExecutionRequest,
) (*workflowservice.DescribeNexusOperationExecutionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DescribeNexusOperationExecution is not supported")
}

func (wh *WorkflowHandler) ListNexusOperationExecutions(
	context.Context,
	*workflowservice.ListNexusOperationExecutionsRequest,
) (*workflowservice.ListNexusOperationExecutionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListNexusOperationExecutions is not supported")
}

func (wh *WorkflowHandler) PauseActivityExecution(
	context.Context,
	*workflowservice.PauseActivityExecutionRequest,
) (*workflowservice.PauseActivityExecutionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "PauseActivityExecution is not supported")
}

func (wh *WorkflowHandler) PollNexusOperationExecution(
	context.Context,
	*workflowservice.PollNexusOperationExecutionRequest,
) (*workflowservice.PollNexusOperationExecutionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "PollNexusOperationExecution is not supported")
}

func (wh *WorkflowHandler) RequestCancelNexusOperationExecution(
	context.Context,
	*workflowservice.RequestCancelNexusOperationExecutionRequest,
) (*workflowservice.RequestCancelNexusOperationExecutionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "RequestCancelNexusOperationExecution is not supported")
}

func (wh *WorkflowHandler) ResetActivityExecution(
	context.Context,
	*workflowservice.ResetActivityExecutionRequest,
) (*workflowservice.ResetActivityExecutionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ResetActivityExecution is not supported")
}

func (wh *WorkflowHandler) StartNexusOperationExecution(
	context.Context,
	*workflowservice.StartNexusOperationExecutionRequest,
) (*workflowservice.StartNexusOperationExecutionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "StartNexusOperationExecution is not supported")
}

func (wh *WorkflowHandler) TerminateNexusOperationExecution(
	context.Context,
	*workflowservice.TerminateNexusOperationExecutionRequest,
) (*workflowservice.TerminateNexusOperationExecutionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "TerminateNexusOperationExecution is not supported")
}

func (wh *WorkflowHandler) UnpauseActivityExecution(
	context.Context,
	*workflowservice.UnpauseActivityExecutionRequest,
) (*workflowservice.UnpauseActivityExecutionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "UnpauseActivityExecution is not supported")
}

func (wh *WorkflowHandler) UpdateActivityExecutionOptions(
	context.Context,
	*workflowservice.UpdateActivityExecutionOptionsRequest,
) (*workflowservice.UpdateActivityExecutionOptionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "UpdateActivityExecutionOptions is not supported")
}
