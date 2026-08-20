package scheduler

import (
	"context"
	"time"

	schedulespb "github.com/superdurable/temporal-server/api/schedule/v1"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/chasm/lib/scheduler/gen/schedulerpb/v1"
	"github.com/superdurable/temporal-server/common/log"
	legacyscheduler "github.com/superdurable/temporal-server/service/worker/scheduler"
)

// Export unexported methods for testing.

func NewTestHandler(logger log.Logger) *handler {
	return newHandler(logger, legacyscheduler.NewSpecBuilder())
}

func (h *handler) TestCreateFromMigrationState(ctx context.Context, req *schedulerpb.CreateFromMigrationStateRequest) (*schedulerpb.CreateFromMigrationStateResponse, error) {
	return h.CreateFromMigrationState(ctx, req)
}

func (h *handler) TestMigrateToWorkflow(ctx context.Context, req *schedulerpb.MigrateToWorkflowRequest) (*schedulerpb.MigrateToWorkflowResponse, error) {
	return h.MigrateToWorkflow(ctx, req)
}

func (s *Scheduler) RecordCompletedAction(
	ctx chasm.MutableContext,
	completed *schedulespb.CompletedResult,
	requestID string,
) time.Time {
	invoker := s.Invoker.Get(ctx)
	return invoker.recordCompletedAction(ctx, completed, requestID)
}

func (i *Invoker) RunningWorkflowID(requestID string) string {
	return i.runningWorkflowID(requestID)
}
