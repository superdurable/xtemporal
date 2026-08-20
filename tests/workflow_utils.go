package tests

import (
	"github.com/superdurable/temporal-server/common/testing/testvars"
	"github.com/superdurable/temporal-server/tests/testcore"
	"go.temporal.io/api/workflowservice/v1"
)

func mustStartWorkflow(s testcore.Env, tv *testvars.TestVars) string {
	s.T().Helper()
	startResp, err := s.FrontendClient().StartWorkflowExecution(testcore.NewContext(), startWorkflowRequest(s, tv))
	if err != nil {
		s.T().Fatalf("Failed to start workflow: %v", err)
	}
	return startResp.GetRunId()
}

func startWorkflowRequest(s testcore.Env, tv *testvars.TestVars) *workflowservice.StartWorkflowExecutionRequest {
	return &workflowservice.StartWorkflowExecutionRequest{
		RequestId:    tv.Any().String(),
		Namespace:    s.Namespace().String(),
		WorkflowId:   tv.WorkflowID(),
		WorkflowType: tv.WorkflowType(),
		TaskQueue:    tv.TaskQueue(),
	}
}
