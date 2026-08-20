package interceptor

import (
	"context"
	"time"

	"github.com/superdurable/temporal-server/common/api"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/common/rpc/interceptor/logtags"
	"github.com/superdurable/temporal-server/common/tasktoken"
	"google.golang.org/grpc"
)

type SlowRequestLoggerInterceptor struct {
	logger               log.Logger
	workflowTags         *logtags.WorkflowTags
	slowRequestThreshold dynamicconfig.DurationPropertyFn
}

func NewSlowRequestLoggerInterceptor(
	logger log.Logger,
	slowRequestThreshold dynamicconfig.DurationPropertyFn,
) *SlowRequestLoggerInterceptor {
	return &SlowRequestLoggerInterceptor{
		logger:               logger,
		workflowTags:         logtags.NewWorkflowTags(tasktoken.NewSerializer(), logger),
		slowRequestThreshold: slowRequestThreshold,
	}
}

func (i *SlowRequestLoggerInterceptor) Intercept(
	ctx context.Context,
	request any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	// Long-polled methods aren't useful logged.
	if api.GetMethodMetadata(info.FullMethod).Polling == api.PollingNone {
		startTime := time.Now()

		defer func() {
			elapsed := time.Since(startTime)
			if elapsed > i.slowRequestThreshold() {
				i.logSlowRequest(request, info, elapsed)
			}
		}()
	}

	return handler(ctx, request)
}

func (i *SlowRequestLoggerInterceptor) logSlowRequest(
	request any,
	info *grpc.UnaryServerInfo,
	elapsed time.Duration,
) {
	method := info.FullMethod

	tags := i.workflowTags.Extract(request, method)
	tags = append(tags, tag.Duration("duration", elapsed))
	tags = append(tags, tag.String("method", method))

	i.logger.Warn("Slow gRPC call", tags...)
}
