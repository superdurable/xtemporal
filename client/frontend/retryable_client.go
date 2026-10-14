package frontend

import (
	"github.com/superdurable/xtemporal/common/backoff"
	"go.temporal.io/api/workflowservice/v1"
)

var _ workflowservice.WorkflowServiceClient = (*retryableClient)(nil)

type retryableClient struct {
	client      workflowservice.WorkflowServiceClient
	policy      backoff.RetryPolicy
	isRetryable backoff.IsRetryable
}

// NewRetryableClient creates a new instance of workflowservice.WorkflowServiceClient with retry policy
func NewRetryableClient(client workflowservice.WorkflowServiceClient, policy backoff.RetryPolicy, isRetryable backoff.IsRetryable) workflowservice.WorkflowServiceClient {
	return &retryableClient{
		client:      client,
		policy:      policy,
		isRetryable: isRetryable,
	}
}
