package paradedb

import (
	enumspb "go.temporal.io/api/enums/v1"
)

// StoreName is the custom datastore name of the Dex-profile ParadeDB visibility store.
const StoreName = "paradedb-dex"

const visibilityTable = "executions_visibility"

// Core columns of schema/paradedb/visibility v2.0.
const (
	colNamespaceID          = "namespace_id"
	colRunID                = "run_id"
	colWorkflowID           = "workflow_id"
	colWorkflowTypeName     = "workflow_type_name"
	colStatus               = "status"
	colStartTime            = "start_time"
	colCloseTime            = "close_time"
	colSortCloseTime        = "sort_close_time"
	colNamespaceDivision    = "namespace_division"
	colFlowType             = "flow_type"
	colTaskQueue            = "task_queue"
	colExecutionTime        = "execution_time"
	colParentWorkflowID     = "parent_workflow_id"
	colParentRunID          = "parent_run_id"
	colRootWorkflowID       = "root_workflow_id"
	colRootRunID            = "root_run_id"
	colHistoryLength        = "history_length"
	colHistorySizeBytes     = "history_size_bytes"
	colExecutionDuration    = "execution_duration"
	colStateTransitionCount = "state_transition_count"
	colMemo                 = "memo"
	colEncoding             = "encoding"
	colVersion              = "_version"
)

// flowTypeSearchAttribute is the Dex search attribute stored in the flow_type column. It is scalar
// only: a list value is rejected with InvalidArgument.
const flowTypeSearchAttribute = "FlowType"

// JSONB columns holding every search attribute that is not a core column, keyed by its real name.
// Datetime values are UTC epoch microseconds; KeywordList values are JSON arrays; nil values and
// empty lists are omitted.
const (
	colSAKeyword  = "sa_keyword"
	colSAText     = "sa_text"
	colSANumber   = "sa_number"
	colSADatetime = "sa_datetime"
	colSABool     = "sa_bool"
)

// searchAttributeColumn returns the JSONB column that stores a search attribute of the given type.
func searchAttributeColumn(t enumspb.IndexedValueType) (string, bool) {
	switch t {
	case enumspb.INDEXED_VALUE_TYPE_KEYWORD, enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST:
		return colSAKeyword, true
	case enumspb.INDEXED_VALUE_TYPE_TEXT:
		return colSAText, true
	case enumspb.INDEXED_VALUE_TYPE_INT, enumspb.INDEXED_VALUE_TYPE_DOUBLE:
		return colSANumber, true
	case enumspb.INDEXED_VALUE_TYPE_DATETIME:
		return colSADatetime, true
	case enumspb.INDEXED_VALUE_TYPE_BOOL:
		return colSABool, true
	default:
		return "", false
	}
}
