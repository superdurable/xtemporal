//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination events_reapplier_mock.go

package ndc

import (
	"context"

	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/serviceerror"
	enumsspb "github.com/superdurable/xtemporal/api/enums/v1"
	chasmworkflow "github.com/superdurable/xtemporal/chasm/lib/workflow"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/service/history/hsm"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/workflow/update"
)

type (
	EventsReapplier interface {
		ReapplyEvents(
			ctx context.Context,
			ms historyi.MutableState,
			updateRegistry update.Registry,
			historyEvents []*historypb.HistoryEvent,
			runID string,
		) ([]*historypb.HistoryEvent, error)
	}

	EventsReapplierImpl struct {
		stateMachineRegistry  *hsm.Registry
		chasmWorkflowRegistry *chasmworkflow.Registry
		metricsHandler        metrics.Handler
		logger                log.Logger
	}
)

func NewEventsReapplier(
	stateMachineRegistry *hsm.Registry,
	chasmWorkflowRegistry *chasmworkflow.Registry,
	metricsHandler metrics.Handler,
	logger log.Logger,
) *EventsReapplierImpl {

	return &EventsReapplierImpl{
		stateMachineRegistry:  stateMachineRegistry,
		chasmWorkflowRegistry: chasmWorkflowRegistry,
		metricsHandler:        metricsHandler,
		logger:                logger,
	}
}

func (r *EventsReapplierImpl) ReapplyEvents(
	ctx context.Context,
	ms historyi.MutableState,
	updateRegistry update.Registry,
	historyEvents []*historypb.HistoryEvent,
	runID string,
) ([]*historypb.HistoryEvent, error) {
	// sanity check workflow still running
	if !ms.IsWorkflowExecutionRunning() {
		return nil, serviceerror.NewInternal("unable to reapply events to closed workflow.")
	}
	reappliedEvents, err := reapplyEvents(ctx, ms, updateRegistry, r.stateMachineRegistry, r.chasmWorkflowRegistry, historyEvents, nil, runID, false, r.logger)
	if err != nil {
		return nil, err
	}
	if len(reappliedEvents) == 0 {
		return nil, nil
	}

	if !ms.IsWorkflowExecutionRunning() {
		// workflow is closed after reapplying events, no need to schedule workflow task
		return reappliedEvents, nil
	}

	// After reapply event, checking if we should schedule a workflow task
	if ms.IsWorkflowPendingOnWorkflowTaskBackoff() {
		// Do not create workflow task when the workflow has first workflow task backoff and execution is not started yet
		return reappliedEvents, nil
	}

	if !ms.HasPendingWorkflowTask() && !ms.IsWorkflowExecutionStatusPaused() {
		if _, err := ms.AddWorkflowTaskScheduledEvent(
			false,
			enumsspb.WORKFLOW_TASK_TYPE_NORMAL,
		); err != nil {
			return nil, err
		}
	}
	return reappliedEvents, nil
}
