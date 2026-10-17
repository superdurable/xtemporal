package paradedb

import (
	"context"

	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store"
)

const upsertVisibilitySQL = `INSERT INTO executions_visibility (
namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, close_time,
namespace_division, flow_type, task_queue, execution_time, parent_workflow_id, parent_run_id,
root_workflow_id, root_run_id, history_length, history_size_bytes, execution_duration,
state_transition_count, memo, encoding, _version, sa_keyword, sa_text, sa_number, sa_datetime, sa_bool
) VALUES (
$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
$14, $15, $16, $17, $18, $19, $20, $21, $22,
$23::jsonb, $24::jsonb, $25::jsonb, $26::jsonb, $27::jsonb
) ON CONFLICT (namespace_id, run_id) DO UPDATE SET
workflow_id = EXCLUDED.workflow_id,
workflow_type_name = EXCLUDED.workflow_type_name,
status = EXCLUDED.status,
start_time = EXCLUDED.start_time,
close_time = EXCLUDED.close_time,
namespace_division = EXCLUDED.namespace_division,
flow_type = EXCLUDED.flow_type,
task_queue = EXCLUDED.task_queue,
execution_time = EXCLUDED.execution_time,
parent_workflow_id = EXCLUDED.parent_workflow_id,
parent_run_id = EXCLUDED.parent_run_id,
root_workflow_id = EXCLUDED.root_workflow_id,
root_run_id = EXCLUDED.root_run_id,
history_length = EXCLUDED.history_length,
history_size_bytes = EXCLUDED.history_size_bytes,
execution_duration = EXCLUDED.execution_duration,
state_transition_count = EXCLUDED.state_transition_count,
memo = EXCLUDED.memo,
encoding = EXCLUDED.encoding,
_version = EXCLUDED._version,
sa_keyword = EXCLUDED.sa_keyword,
sa_text = EXCLUDED.sa_text,
sa_number = EXCLUDED.sa_number,
sa_datetime = EXCLUDED.sa_datetime,
sa_bool = EXCLUDED.sa_bool
WHERE executions_visibility._version < EXCLUDED._version`

func (s *VisibilityStore) RecordWorkflowExecutionStarted(ctx context.Context, request *store.InternalRecordWorkflowExecutionStartedRequest) error {
	return s.writeExecution(ctx, request.InternalVisibilityRequestBase, nil)
}

func (s *VisibilityStore) RecordWorkflowExecutionClosed(ctx context.Context, request *store.InternalRecordWorkflowExecutionClosedRequest) error {
	return s.writeExecution(ctx, request.InternalVisibilityRequestBase, request)
}

func (s *VisibilityStore) UpsertWorkflowExecution(ctx context.Context, request *store.InternalUpsertWorkflowExecutionRequest) error {
	return s.writeExecution(ctx, request.InternalVisibilityRequestBase, nil)
}

func (s *VisibilityStore) writeExecution(ctx context.Context, request *store.InternalVisibilityRequestBase, closed *store.InternalRecordWorkflowExecutionClosedRequest) error {
	attrs, err := s.encodeRequestAttributes(request.SearchAttributes)
	if err != nil {
		return err
	}
	row := visibilityRow{
		NamespaceID: request.NamespaceID, RunID: request.RunID, WorkflowID: request.WorkflowID,
		WorkflowTypeName: request.WorkflowTypeName, Status: int32(request.Status),
		StartTime: request.StartTime.UTC(), NamespaceDivision: attrs.division, FlowType: attrs.flowType,
		TaskQueue: request.TaskQueue, ExecutionTime: request.ExecutionTime.UTC(),
		ParentWorkflowID: request.ParentWorkflowID, ParentRunID: request.ParentRunID,
		RootWorkflowID: request.RootWorkflowID, RootRunID: request.RootRunID,
		Version: request.TaskID, SAKeyword: attrs.keyword, SAText: attrs.text,
		SANumber: attrs.number, SADatetime: attrs.datetime, SABool: attrs.boolean,
	}
	if request.Memo != nil {
		row.Memo = request.Memo.Data
		row.Encoding = request.Memo.EncodingType.String()
	} else {
		row.Encoding = "json/plain"
	}
	if closed != nil {
		closeTime := closed.CloseTime.UTC()
		row.CloseTime = &closeTime
		row.HistoryLength = &closed.HistoryLength
		row.HistorySizeBytes = &closed.HistorySizeBytes
		duration := closed.ExecutionDuration.Nanoseconds()
		row.ExecutionDuration = &duration
		row.StateTransitionCount = &closed.StateTransitionCount
	}
	_, err = s.btree.ExecContext(ctx, upsertVisibilitySQL,
		row.NamespaceID, row.RunID, row.WorkflowID, row.WorkflowTypeName, row.Status,
		row.StartTime, row.CloseTime, row.NamespaceDivision, row.FlowType, row.TaskQueue,
		row.ExecutionTime, row.ParentWorkflowID, row.ParentRunID, row.RootWorkflowID, row.RootRunID,
		row.HistoryLength, row.HistorySizeBytes, row.ExecutionDuration, row.StateTransitionCount,
		row.Memo, row.Encoding, row.Version, string(row.SAKeyword), string(row.SAText),
		string(row.SANumber), string(row.SADatetime), string(row.SABool),
	)
	return visibilityWriteError("write ParadeDB visibility execution", err)
}

func (s *VisibilityStore) DeleteWorkflowExecution(ctx context.Context, request *manager.VisibilityDeleteWorkflowExecutionRequest) error {
	_, err := s.btree.ExecContext(ctx,
		`DELETE FROM executions_visibility WHERE namespace_id = $1 AND run_id = $2 AND _version <= $3`,
		request.NamespaceID.String(), request.RunID, request.TaskID,
	)
	return visibilityWriteError("delete ParadeDB visibility execution", err)
}
