package operator

import (
	"github.com/superdurable/xtemporal/common/backoff"
	"go.temporal.io/api/operatorservice/v1"
)

var _ operatorservice.OperatorServiceClient = (*retryableClient)(nil)

type retryableClient struct {
	client      operatorservice.OperatorServiceClient
	policy      backoff.RetryPolicy
	isRetryable backoff.IsRetryable
}

// NewRetryableClient creates a new instance of operatorservice.OperatorServiceClient with retry policy
func NewRetryableClient(client operatorservice.OperatorServiceClient, policy backoff.RetryPolicy, isRetryable backoff.IsRetryable) operatorservice.OperatorServiceClient {
	return &retryableClient{
		client:      client,
		policy:      policy,
		isRetryable: isRetryable,
	}
}
