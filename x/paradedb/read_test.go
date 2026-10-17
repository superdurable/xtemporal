package paradedb

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

func TestBoundedNeedsFallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scanned int64
		matches int
		want    bool
	}{
		{"count only before bound", 999, 0, false},
		{"count only at bound", 1000, 0, true},
		{"partial page before bound", 999, 1, false},
		{"partial page at bound", 1000, 1, true},
		{"full page at bound", 1000, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, boundedNeedsFallback(tc.scanned, tc.matches, 2, 1000))
		})
	}
}

func TestReadErrors(t *testing.T) {
	s := &VisibilityStore{}
	_, err := s.chasmMapper(123)
	require.ErrorAs(t, err, new(*serviceerror.InvalidArgument))
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		require.ErrorIs(t, visibilityWriteError("read", cause), cause)
	}
	err = visibilityWriteError("read", errors.New("database failed"))
	require.ErrorAs(t, err, new(*serviceerror.Unavailable))
}

func TestBoundedExecutorIntegration(t *testing.T) {
	port, dbName := paradeDBTestDatabase(t)
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, paradeDBTestDSN(port, dbName))
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close(ctx)) }()
	seedParadeDB(ctx, t, conn)
	cfg := config.CustomDatastoreConfig{Name: StoreName, IndexName: "dex_visibility", Options: map[string]any{
		"sql": map[string]any{"connectAddr": "127.0.0.1:" + port, "databaseName": dbName,
			"user": "temporal", "password": "temporal"},
	}}
	opened, err := (VisibilityStoreFactory{}).NewVisibilityStore(cfg, integrationProvider{}, searchattribute.NewTestMapperProvider(nil), nil, nil, nil, nil, nil)
	require.NoError(t, err)
	s := opened.(*VisibilityStore)
	defer s.Close()
	grouped, err := s.CountWorkflowExecutions(ctx, &manager.CountWorkflowExecutionsRequest{
		NamespaceID: namespace.ID("ns-a"), Namespace: namespace.Name("test"), Query: "GROUP BY TemporalNamespaceDivision",
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), grouped.Count)
	countsByDivision := map[string]int64{}
	for _, group := range grouped.Groups {
		value, decodeErr := sadefs.DecodeValue(group.GroupValues[0], enumspb.INDEXED_VALUE_TYPE_KEYWORD, false)
		require.NoError(t, decodeErr)
		countsByDivision[value.(string)] = group.Count
	}
	require.Equal(t, map[string]int64{"": 4, "TemporalScheduler": 1}, countsByDivision)
	for _, tc := range []struct {
		name, query string
		want        []string
	}{
		{"matching rows", `FlowType = 'approval' AND KeywordField = 'red'`, []string{"r5", "r1"}},
		{"count only row", `FlowType = 'approval' AND KeywordField = 'absent'`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, convertErr := testConvert(t, tc.query, chasm.UnspecifiedArchetypeID)
			require.NoError(t, convertErr)
			plan, renderErr := renderList(q, "ns-a", 2, nil)
			require.NoError(t, renderErr)
			require.NotNil(t, plan.Bounded)
			rows, readErr := s.executeBounded(ctx, plan.Bounded, 2)
			require.NoError(t, readErr)
			runIDs := make([]string, 0, len(rows))
			for _, row := range rows {
				runIDs = append(runIDs, row.RunID)
			}
			require.ElementsMatch(t, tc.want, runIDs)
		})
	}
	_, err = conn.Exec(ctx, `INSERT INTO executions_visibility
	(namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, close_time, flow_type, sa_keyword)
	SELECT 'ns-bound', 'candidate-' || g::text, 'candidate', 'Engine', 1, '2026-01-02'::timestamptz + g * interval '1 second',
	'2026-01-02'::timestamptz + g * interval '1 second', '2026-01-02'::timestamptz + g * interval '1 second',
	'approval', '{"KeywordField":"common"}'::jsonb FROM generate_series(1, 1001) AS g`)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO executions_visibility
	(namespace_id, run_id, workflow_id, workflow_type_name, status, start_time, execution_time, close_time, flow_type, sa_keyword)
	VALUES ('ns-bound','target','target','Engine',1,'2026-01-01','2026-01-01','2026-01-01','approval','{"KeywordField":"rare"}')`)
	require.NoError(t, err)
	q, err := testConvert(t, `FlowType = 'approval' AND KeywordField = 'rare'`, chasm.UnspecifiedArchetypeID)
	require.NoError(t, err)
	plan, err := renderList(q, "ns-bound", 1, nil)
	require.NoError(t, err)
	rows, err := s.executeBounded(ctx, plan.Bounded, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "target", rows[0].RunID)
}
