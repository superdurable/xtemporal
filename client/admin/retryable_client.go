package admin

import (
	"context"

	"github.com/superdurable/temporal-server/api/adminservice/v1"
	"github.com/superdurable/temporal-server/common/backoff"
	"google.golang.org/grpc"
)

var _ adminservice.AdminServiceClient = (*retryableClient)(nil)

type retryableClient struct {
	client      adminservice.AdminServiceClient
	policy      backoff.RetryPolicy
	isRetryable backoff.IsRetryable
}

// NewRetryableClient creates a new instance of adminservice.AdminServiceClient with retry policy
func NewRetryableClient(client adminservice.AdminServiceClient, policy backoff.RetryPolicy, isRetryable backoff.IsRetryable) adminservice.AdminServiceClient {
	return &retryableClient{
		client:      client,
		policy:      policy,
		isRetryable: isRetryable,
	}
}

func (c *retryableClient) StreamWorkflowReplicationMessages(
	ctx context.Context,
	opts ...grpc.CallOption,
) (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
	var resp adminservice.AdminService_StreamWorkflowReplicationMessagesClient
	op := func(ctx context.Context) error {
		var err error
		resp, err = c.client.StreamWorkflowReplicationMessages(ctx, opts...)
		return err
	}
	err := backoff.ThrottleRetryContext(ctx, op, c.policy, c.isRetryable)
	return resp, err
}
