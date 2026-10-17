package paradedb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	visibilityservicespb "github.com/superdurable/xtemporal/api/visibilityservice/v1"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

type readRequest struct {
	namespaceID namespace.ID
	namespace   namespace.Name
	query       string
	pageSize    int
	token       []byte
	chasmMapper *chasm.VisibilitySearchAttributesMapper
	archetypeID chasm.ArchetypeID
}

func (s *VisibilityStore) ListWorkflowExecutions(ctx context.Context, request *manager.ListWorkflowExecutionsRequestV2) (*store.InternalListExecutionsResponse, error) {
	return s.listExecutions(ctx, readRequest{namespaceID: request.NamespaceID, namespace: request.Namespace,
		query: request.Query, pageSize: request.PageSize, token: request.NextPageToken,
		archetypeID: chasm.UnspecifiedArchetypeID})
}

func (s *VisibilityStore) CountWorkflowExecutions(ctx context.Context, request *manager.CountWorkflowExecutionsRequest) (*store.InternalCountExecutionsResponse, error) {
	return s.countExecutions(ctx, readRequest{namespaceID: request.NamespaceID, namespace: request.Namespace,
		query: request.Query, archetypeID: chasm.UnspecifiedArchetypeID})
}

func (s *VisibilityStore) ListChasmExecutions(ctx context.Context, request *visibilityservicespb.ListChasmExecutionsRequest) (*store.InternalListExecutionsResponse, error) {
	mapper, err := s.chasmMapper(request.ArchetypeId)
	if err != nil {
		return nil, err
	}
	return s.listExecutions(ctx, readRequest{namespaceID: namespace.ID(request.NamespaceId), namespace: namespace.Name(request.Namespace),
		query: request.Query, pageSize: int(request.PageSize), token: request.NextPageToken,
		chasmMapper: mapper, archetypeID: request.ArchetypeId})
}

func (s *VisibilityStore) CountChasmExecutions(ctx context.Context, request *visibilityservicespb.CountChasmExecutionsRequest) (*store.InternalCountExecutionsResponse, error) {
	mapper, err := s.chasmMapper(request.ArchetypeId)
	if err != nil {
		return nil, err
	}
	return s.countExecutions(ctx, readRequest{namespaceID: namespace.ID(request.NamespaceId), namespace: namespace.Name(request.Namespace),
		query: request.Query, chasmMapper: mapper, archetypeID: request.ArchetypeId})
}

func (s *VisibilityStore) chasmMapper(id chasm.ArchetypeID) (*chasm.VisibilitySearchAttributesMapper, error) {
	if s.chasmRegistry == nil {
		return nil, serviceerror.NewInvalidArgumentf("unknown archetype ID: %d", id)
	}
	rc, ok := s.chasmRegistry.ComponentByID(id)
	if !ok {
		return nil, serviceerror.NewInvalidArgumentf("unknown archetype ID: %d", id)
	}
	return rc.SearchAttributesMapper(), nil
}

func (s *VisibilityStore) convertReadQuery(req readRequest) (*convertedQuery, searchattribute.NameTypeMap, error) {
	types, err := s.searchAttributeTypes()
	if err != nil {
		return nil, types, err
	}
	mapper, err := s.mapperProvider.GetMapper(req.namespace)
	if err != nil {
		return nil, types, err
	}
	query, err := convertQuery(req.query, req.namespace, types, mapper, req.chasmMapper,
		req.archetypeID, s.metricsHandler, s.logger)
	return query, types, err
}

func (s *VisibilityStore) listExecutions(ctx context.Context, req readRequest) (*store.InternalListExecutionsResponse, error) {
	query, types, err := s.convertReadQuery(req)
	if err != nil {
		return nil, err
	}
	plan, err := renderList(query, req.namespaceID.String(), req.pageSize, req.token)
	if err != nil {
		return nil, err
	}
	db := s.dbForRoute(plan.Route)
	var rows []visibilityRow
	if plan.Bounded != nil {
		rows, err = s.executeBounded(ctx, plan.Bounded, req.pageSize)
	} else {
		err = db.SelectContext(ctx, &rows, plan.SQL, plan.Args...)
	}
	if err != nil {
		return nil, visibilityWriteError("list ParadeDB visibility executions", err)
	}
	response := &store.InternalListExecutionsResponse{Executions: make([]*store.InternalExecutionInfo, 0, len(rows))}
	for i := range rows {
		info, decodeErr := rowToInfo(&rows[i], types)
		if decodeErr != nil {
			return nil, serviceerror.NewInternalf("decode ParadeDB visibility execution: %v", decodeErr)
		}
		response.Executions = append(response.Executions, info)
	}
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		response.NextPageToken, err = nextPageToken(len(rows), req.pageSize, last.SortCloseTime, last.StartTime, last.RunID)
		if err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (s *VisibilityStore) dbForRoute(r route) *sqlx.DB {
	if r == routeBM25 {
		return s.bm25
	}
	return s.btree
}

func (s *VisibilityStore) countExecutions(ctx context.Context, req readRequest) (*store.InternalCountExecutionsResponse, error) {
	query, _, err := s.convertReadQuery(req)
	if err != nil {
		return nil, err
	}
	plan, err := renderCount(query, req.namespaceID.String())
	if err != nil {
		return nil, err
	}
	db := s.dbForRoute(plan.Route)
	response := &store.InternalCountExecutionsResponse{}
	switch plan.GroupBy {
	case "":
		err = db.GetContext(ctx, &response.Count, plan.SQL, plan.Args...)
	case sadefs.ExecutionStatus:
		var groups []struct {
			Status int32 `db:"status"`
			Count  int64 `db:"count"`
		}
		err = db.SelectContext(ctx, &groups, plan.SQL, plan.Args...)
		if err == nil {
			for _, group := range groups {
				value := enumspb.WorkflowExecutionStatus(group.Status).String()
				if err = appendCountGroup(response, value, enumspb.INDEXED_VALUE_TYPE_KEYWORD, group.Count); err != nil {
					return nil, err
				}
			}
		}
	case sadefs.TemporalNamespaceDivision:
		var groups []struct {
			Division string `db:"namespace_division"`
			Count    int64  `db:"count"`
		}
		err = db.SelectContext(ctx, &groups, plan.SQL, plan.Args...)
		if err == nil {
			for _, group := range groups {
				if err = appendCountGroup(response, group.Division, enumspb.INDEXED_VALUE_TYPE_KEYWORD, group.Count); err != nil {
					return nil, err
				}
			}
		}
	default:
		return nil, serviceerror.NewInvalidArgumentf("unsupported GROUP BY %s", plan.GroupBy)
	}
	if err != nil {
		return nil, visibilityWriteError("count ParadeDB visibility executions", err)
	}
	return response, nil
}

func appendCountGroup(response *store.InternalCountExecutionsResponse, value string, typ enumspb.IndexedValueType, count int64) error {
	payload, err := sadefs.EncodeValue(value, typ)
	if err != nil {
		return fmt.Errorf("encode ParadeDB count group: %w", err)
	}
	response.Groups = append(response.Groups, store.InternalAggregationGroup{GroupValues: []*commonpb.Payload{payload}, Count: count})
	response.Count += count
	return nil
}

type boundedVisibilityRow struct {
	NamespaceID          sql.NullString `db:"namespace_id"`
	RunID                sql.NullString `db:"run_id"`
	WorkflowID           sql.NullString `db:"workflow_id"`
	WorkflowTypeName     sql.NullString `db:"workflow_type_name"`
	Status               sql.NullInt64  `db:"status"`
	StartTime            sql.NullTime   `db:"start_time"`
	CloseTime            *time.Time     `db:"close_time"`
	SortCloseTime        sql.NullTime   `db:"sort_close_time"`
	NamespaceDivision    *string        `db:"namespace_division"`
	FlowType             *string        `db:"flow_type"`
	TaskQueue            sql.NullString `db:"task_queue"`
	ExecutionTime        sql.NullTime   `db:"execution_time"`
	ParentWorkflowID     *string        `db:"parent_workflow_id"`
	ParentRunID          *string        `db:"parent_run_id"`
	RootWorkflowID       sql.NullString `db:"root_workflow_id"`
	RootRunID            sql.NullString `db:"root_run_id"`
	HistoryLength        *int64         `db:"history_length"`
	HistorySizeBytes     *int64         `db:"history_size_bytes"`
	ExecutionDuration    *int64         `db:"execution_duration"`
	StateTransitionCount *int64         `db:"state_transition_count"`
	Memo                 []byte         `db:"memo"`
	Encoding             sql.NullString `db:"encoding"`
	Version              sql.NullInt64  `db:"_version"`
	SAKeyword            []byte         `db:"sa_keyword"`
	SAText               []byte         `db:"sa_text"`
	SANumber             []byte         `db:"sa_number"`
	SADatetime           []byte         `db:"sa_datetime"`
	SABool               []byte         `db:"sa_bool"`
	ScannedCount         int64          `db:"scanned_count"`
}

func (r *boundedVisibilityRow) visibilityRow() visibilityRow {
	return visibilityRow{NamespaceID: r.NamespaceID.String, RunID: r.RunID.String, WorkflowID: r.WorkflowID.String,
		WorkflowTypeName: r.WorkflowTypeName.String, Status: int32(r.Status.Int64), StartTime: r.StartTime.Time,
		CloseTime: r.CloseTime, SortCloseTime: r.SortCloseTime.Time, NamespaceDivision: r.NamespaceDivision,
		FlowType: r.FlowType, TaskQueue: r.TaskQueue.String, ExecutionTime: r.ExecutionTime.Time,
		ParentWorkflowID: r.ParentWorkflowID, ParentRunID: r.ParentRunID, RootWorkflowID: r.RootWorkflowID.String,
		RootRunID: r.RootRunID.String, HistoryLength: r.HistoryLength, HistorySizeBytes: r.HistorySizeBytes,
		ExecutionDuration: r.ExecutionDuration, StateTransitionCount: r.StateTransitionCount, Memo: r.Memo,
		Encoding: r.Encoding.String, Version: r.Version.Int64, SAKeyword: r.SAKeyword, SAText: r.SAText,
		SANumber: r.SANumber, SADatetime: r.SADatetime, SABool: r.SABool}
}

func boundedNeedsFallback(scanned int64, matches, pageSize, bound int) bool {
	return scanned == int64(bound) && matches < pageSize
}

func (s *VisibilityStore) executeBounded(ctx context.Context, plan *boundedPlan, pageSize int) ([]visibilityRow, error) {
	var bounded []boundedVisibilityRow
	if err := s.btree.SelectContext(ctx, &bounded, plan.SQL, plan.Args...); err != nil {
		return nil, err
	}
	rows := make([]visibilityRow, 0, len(bounded))
	var scanned int64
	for i := range bounded {
		scanned = bounded[i].ScannedCount
		if bounded[i].RunID.Valid {
			rows = append(rows, bounded[i].visibilityRow())
		}
	}
	if boundedNeedsFallback(scanned, len(rows), pageSize, plan.Bound) {
		if err := s.bm25.SelectContext(ctx, &rows, plan.FallbackSQL, plan.FallbackArgs...); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
