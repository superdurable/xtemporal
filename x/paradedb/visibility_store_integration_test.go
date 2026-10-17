package paradedb

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

type integrationProvider struct{}

func (integrationProvider) GetSearchAttributes(string, bool) (searchattribute.NameTypeMap, error) {
	return codecTypes(), nil
}

func TestParadeDBVisibilityStoreIntegration(t *testing.T) {
	if os.Getenv("PARADEDB_TEST") != "1" {
		t.Skip("set PARADEDB_TEST=1 to run ParadeDB integration test")
	}
	port := os.Getenv("POSTGRES_PORT")
	require.NotEmpty(t, port)
	cfg := config.CustomDatastoreConfig{
		Name: StoreName, IndexName: "dex_visibility",
		Options: map[string]any{"profile": "dex", "sql": map[string]any{
			"pluginName": "postgres12_pgx", "connectAddr": "127.0.0.1:" + port,
			"databaseName": "visibility", "connectProtocol": "tcp",
			"user": "postgres", "password": "postgres", "maxConns": 4,
		}},
	}
	opened, err := (VisibilityStoreFactory{}).NewVisibilityStore(cfg, integrationProvider{}, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	s := opened.(*VisibilityStore)
	t.Cleanup(s.Close)
	ctx := context.Background()
	_, err = s.btree.ExecContext(ctx, `TRUNCATE executions_visibility`)
	require.NoError(t, err)

	var jit, planMode string
	require.NoError(t, s.btree.GetContext(ctx, &jit, `SHOW jit`))
	require.Equal(t, "off", jit)
	require.NoError(t, s.btree.GetContext(ctx, &planMode, `SHOW plan_cache_mode`))
	require.Equal(t, "auto", planMode)
	require.NoError(t, s.bm25.GetContext(ctx, &planMode, `SHOW plan_cache_mode`))
	require.Equal(t, "force_custom_plan", planMode)

	start := time.Date(2025, 4, 5, 6, 7, 8, 0, time.UTC)
	attrs := &commonpb.SearchAttributes{IndexedFields: map[string]*commonpb.Payload{}}
	for name, value := range map[string]any{
		"FlowType": "invoice", sadefs.TemporalNamespaceDivision: "TemporalScheduler",
		"Keyword": []string{"a", "b"}, "Keywords": []string{"x", "y"},
		"Text": []string{"hello", "world"}, "Int": []int64{math.MaxInt64, -1},
		"Double": []float64{1.25, 2.5}, "Datetime": []time.Time{start},
		"Bool": []bool{true, false}, "TemporalInt01": int64(9),
	} {
		typeID, typeErr := searchAttributeType(codecTypes(), name)
		require.NoError(t, typeErr)
		attrs.IndexedFields[name], err = sadefs.EncodeValue(value, typeID)
		require.NoError(t, err)
	}
	parent := "parent"
	base := &store.InternalVisibilityRequestBase{
		NamespaceID: "namespace", RunID: "run", WorkflowID: "workflow", WorkflowTypeName: "type",
		StartTime: start, ExecutionTime: start, Status: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
		TaskID: 10, TaskQueue: "queue", SearchAttributes: attrs,
		ParentWorkflowID: &parent, RootWorkflowID: "root", RootRunID: "root-run",
		Memo: &commonpb.DataBlob{Data: []byte("memo"), EncodingType: enumspb.ENCODING_TYPE_PROTO3},
	}
	require.NoError(t, s.RecordWorkflowExecutionStarted(ctx, &store.InternalRecordWorkflowExecutionStartedRequest{InternalVisibilityRequestBase: base}))
	read := func() *store.InternalExecutionInfo {
		response, readErr := s.GetWorkflowExecution(ctx, &manager.GetWorkflowExecutionRequest{NamespaceID: namespace.ID("namespace"), RunID: "run"})
		require.NoError(t, readErr)
		return response.Execution
	}
	require.Equal(t, enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, read().Status)
	upsert := *base
	upsert.TaskID = 11
	upsert.TaskQueue = "updated"
	require.NoError(t, s.UpsertWorkflowExecution(ctx, &store.InternalUpsertWorkflowExecutionRequest{InternalVisibilityRequestBase: &upsert}))
	require.Equal(t, "updated", read().TaskQueue)
	stale := *base
	stale.TaskID = 11
	stale.TaskQueue = "stale"
	require.NoError(t, s.UpsertWorkflowExecution(ctx, &store.InternalUpsertWorkflowExecutionRequest{InternalVisibilityRequestBase: &stale}))
	stale.TaskID = 9
	require.NoError(t, s.UpsertWorkflowExecution(ctx, &store.InternalUpsertWorkflowExecutionRequest{InternalVisibilityRequestBase: &stale}))
	require.Equal(t, "updated", read().TaskQueue)
	require.NoError(t, s.DeleteWorkflowExecution(ctx, &manager.VisibilityDeleteWorkflowExecutionRequest{NamespaceID: namespace.ID("namespace"), RunID: "run", TaskID: 10}))
	require.Equal(t, "updated", read().TaskQueue)
	closedBase := upsert
	closedBase.TaskID = 12
	closedBase.Status = enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED
	closeTime := start.Add(time.Minute)
	require.NoError(t, s.RecordWorkflowExecutionClosed(ctx, &store.InternalRecordWorkflowExecutionClosedRequest{
		InternalVisibilityRequestBase: &closedBase, CloseTime: closeTime,
		HistoryLength: 13, HistorySizeBytes: 14, ExecutionDuration: time.Minute,
		StateTransitionCount: 15,
	}))
	info := read()
	require.Equal(t, enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED, info.Status)
	require.Equal(t, closeTime, info.CloseTime)
	require.Equal(t, int64(13), info.HistoryLength)
	require.Equal(t, int64(14), info.HistorySizeBytes)
	require.Equal(t, int64(15), info.StateTransitionCount)
	require.Equal(t, time.Minute, info.ExecutionDuration)
	require.Equal(t, "parent", info.ParentWorkflowID)
	require.Equal(t, "root", info.RootWorkflowID)
	require.Equal(t, []byte("memo"), info.Memo.Data)
	require.Equal(t, enumspb.ENCODING_TYPE_PROTO3, info.Memo.EncodingType)
	for name, want := range map[string]any{
		"FlowType": "invoice", sadefs.TemporalNamespaceDivision: "TemporalScheduler",
		"Keyword": []string{"a", "b"}, "Keywords": []string{"x", "y"},
		"Text": []string{"hello", "world"}, "Int": []int64{math.MaxInt64, -1},
		"Double": []float64{1.25, 2.5}, "Datetime": []time.Time{start},
		"Bool": []bool{true, false}, "TemporalInt01": int64(9),
	} {
		typeID, typeErr := searchAttributeType(codecTypes(), name)
		require.NoError(t, typeErr)
		got, decodeErr := sadefs.DecodeValue(info.SearchAttributes.IndexedFields[name], typeID, true)
		require.NoError(t, decodeErr)
		require.Equal(t, want, got)
	}
	var flowType, division string
	require.NoError(t, s.btree.QueryRowContext(ctx, `SELECT flow_type, namespace_division FROM executions_visibility WHERE run_id = 'run'`).Scan(&flowType, &division))
	require.Equal(t, "invoice", flowType)
	require.Equal(t, "TemporalScheduler", division)
	var flowInKeyword bool
	require.NoError(t, s.btree.GetContext(ctx, &flowInKeyword, `SELECT sa_keyword ? 'FlowType' FROM executions_visibility WHERE run_id = 'run'`))
	require.False(t, flowInKeyword)

	badFlow, encodeErr := sadefs.EncodeValue([]string{"one"}, enumspb.INDEXED_VALUE_TYPE_KEYWORD)
	require.NoError(t, encodeErr)
	badBase := closedBase
	badBase.TaskID = 13
	badAttrs := &commonpb.SearchAttributes{IndexedFields: map[string]*commonpb.Payload{"FlowType": badFlow}}
	badBase.SearchAttributes = badAttrs
	err = s.UpsertWorkflowExecution(ctx, &store.InternalUpsertWorkflowExecutionRequest{InternalVisibilityRequestBase: &badBase})
	require.ErrorAs(t, err, new(*serviceerror.InvalidArgument))

	require.NoError(t, s.DeleteWorkflowExecution(ctx, &manager.VisibilityDeleteWorkflowExecutionRequest{NamespaceID: namespace.ID("namespace"), RunID: "run", TaskID: math.MaxInt64}))
	_, err = s.GetWorkflowExecution(ctx, &manager.GetWorkflowExecutionRequest{NamespaceID: namespace.ID("namespace"), RunID: "run"})
	require.ErrorAs(t, err, new(*serviceerror.NotFound))

	_, err = s.btree.ExecContext(ctx, `UPDATE schema_version SET curr_version = '1.14' WHERE db_name = 'visibility'`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, restoreErr := s.btree.ExecContext(context.Background(), `UPDATE schema_version SET curr_version = '2.0' WHERE db_name = 'visibility'`)
		require.NoError(t, restoreErr)
	})
	_, err = (VisibilityStoreFactory{}).NewVisibilityStore(cfg, integrationProvider{}, nil, nil, nil, nil, nil, nil)
	require.ErrorContains(t, err, "requires schema version 2.0")
}
