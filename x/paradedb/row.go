package paradedb

import (
	"time"

	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store"
	"github.com/superdurable/xtemporal/common/searchattribute"
	enumspb "go.temporal.io/api/enums/v1"
)

type visibilityRow struct {
	NamespaceID          string     `db:"namespace_id"`
	RunID                string     `db:"run_id"`
	WorkflowID           string     `db:"workflow_id"`
	WorkflowTypeName     string     `db:"workflow_type_name"`
	Status               int32      `db:"status"`
	StartTime            time.Time  `db:"start_time"`
	CloseTime            *time.Time `db:"close_time"`
	SortCloseTime        time.Time  `db:"sort_close_time"`
	NamespaceDivision    *string    `db:"namespace_division"`
	FlowType             *string    `db:"flow_type"`
	TaskQueue            string     `db:"task_queue"`
	ExecutionTime        time.Time  `db:"execution_time"`
	ParentWorkflowID     *string    `db:"parent_workflow_id"`
	ParentRunID          *string    `db:"parent_run_id"`
	RootWorkflowID       string     `db:"root_workflow_id"`
	RootRunID            string     `db:"root_run_id"`
	HistoryLength        *int64     `db:"history_length"`
	HistorySizeBytes     *int64     `db:"history_size_bytes"`
	ExecutionDuration    *int64     `db:"execution_duration"`
	StateTransitionCount *int64     `db:"state_transition_count"`
	Memo                 []byte     `db:"memo"`
	Encoding             string     `db:"encoding"`
	Version              int64      `db:"_version"`
	SAKeyword            []byte     `db:"sa_keyword"`
	SAText               []byte     `db:"sa_text"`
	SANumber             []byte     `db:"sa_number"`
	SADatetime           []byte     `db:"sa_datetime"`
	SABool               []byte     `db:"sa_bool"`
}

func rowToInfo(row *visibilityRow, types searchattribute.NameTypeMap) (*store.InternalExecutionInfo, error) {
	attrs, err := decodeSearchAttributes(encodedSearchAttributes{
		keyword: row.SAKeyword, text: row.SAText, number: row.SANumber,
		datetime: row.SADatetime, boolean: row.SABool,
		flowType: row.FlowType, division: row.NamespaceDivision,
	}, types)
	if err != nil {
		return nil, err
	}
	info := &store.InternalExecutionInfo{
		WorkflowID: row.WorkflowID, RunID: row.RunID, TypeName: row.WorkflowTypeName,
		StartTime: row.StartTime.UTC(), ExecutionTime: row.ExecutionTime.UTC(),
		Status:    enumspb.WorkflowExecutionStatus(row.Status),
		TaskQueue: row.TaskQueue, RootWorkflowID: row.RootWorkflowID, RootRunID: row.RootRunID,
		Memo: persistence.NewDataBlob(row.Memo, row.Encoding), SearchAttributes: attrs,
	}
	if row.CloseTime != nil {
		info.CloseTime = row.CloseTime.UTC()
	}
	if row.ExecutionDuration != nil {
		info.ExecutionDuration = time.Duration(*row.ExecutionDuration)
	}
	if row.HistoryLength != nil {
		info.HistoryLength = *row.HistoryLength
	}
	if row.HistorySizeBytes != nil {
		info.HistorySizeBytes = *row.HistorySizeBytes
	}
	if row.StateTransitionCount != nil {
		info.StateTransitionCount = *row.StateTransitionCount
	}
	if row.ParentWorkflowID != nil {
		info.ParentWorkflowID = *row.ParentWorkflowID
	}
	if row.ParentRunID != nil {
		info.ParentRunID = *row.ParentRunID
	}
	return info, nil
}
