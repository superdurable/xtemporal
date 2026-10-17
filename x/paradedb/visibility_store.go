package paradedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/common/persistence/visibility/store"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/serviceerror"
)

type VisibilityStore struct {
	btree             *sqlx.DB
	bm25              *sqlx.DB
	indexName         string
	saProvider        searchattribute.Provider
	mapperProvider    searchattribute.MapperProvider
	namespaceRegistry namespace.Registry
	chasmRegistry     *chasm.Registry
	metricsHandler    metrics.Handler
	logger            log.Logger
}

var _ store.VisibilityStore = (*VisibilityStore)(nil)

func (s *VisibilityStore) Close() {
	_ = s.btree.Close()
	_ = s.bm25.Close()
}

func (*VisibilityStore) GetName() string        { return StoreName }
func (s *VisibilityStore) GetIndexName() string { return s.indexName }

func (s *VisibilityStore) searchAttributeTypes() (searchattribute.NameTypeMap, error) {
	types, err := s.saProvider.GetSearchAttributes(s.indexName, false)
	if err != nil {
		return types, serviceerror.NewUnavailablef("unable to read search attribute types: %v", err)
	}
	return types, nil
}

func (s *VisibilityStore) ValidateCustomSearchAttributes(values map[string]any) (map[string]any, error) {
	validated := make(map[string]any, len(values))
	types, err := s.searchAttributeTypes()
	if err != nil {
		return validated, err
	}
	for name, value := range values {
		t, err := searchAttributeType(types, name)
		if err != nil {
			return validated, serviceerror.NewInvalidArgumentf("unregistered search attribute %q", name)
		}
		if (name == flowTypeSearchAttribute || name == sadefs.TemporalNamespaceDivision) && value != nil {
			if _, ok := value.(string); !ok {
				return validated, serviceerror.NewInvalidArgumentf("%s must be a scalar keyword", name)
			}
		}
		if value == nil || isEmptyList(value) {
			continue
		}
		if _, err := normalizeSAValue(value, t); err != nil {
			return validated, serviceerror.NewInvalidArgumentf("invalid search attribute %q: %v", name, err)
		}
		validated[name] = value
	}
	return validated, nil
}

func (s *VisibilityStore) GetWorkflowExecution(ctx context.Context, request *manager.GetWorkflowExecutionRequest) (*store.InternalGetWorkflowExecutionResponse, error) {
	var row visibilityRow
	err := s.btree.GetContext(ctx, &row, `SELECT namespace_id, run_id, workflow_id, workflow_type_name, status,
start_time, close_time, sort_close_time, namespace_division, flow_type, task_queue, execution_time,
parent_workflow_id, parent_run_id, root_workflow_id, root_run_id, history_length, history_size_bytes,
execution_duration, state_transition_count, memo, encoding, _version, sa_keyword, sa_text,
sa_number, sa_datetime, sa_bool FROM executions_visibility WHERE namespace_id = $1 AND run_id = $2`, request.NamespaceID.String(), request.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, serviceerror.NewNotFoundf("workflow execution with RunId %s not found", request.RunID)
	}
	if err != nil {
		return nil, visibilityWriteError("get ParadeDB visibility execution", err)
	}
	types, err := s.searchAttributeTypes()
	if err != nil {
		return nil, err
	}
	info, err := rowToInfo(&row, types)
	if err != nil {
		return nil, serviceerror.NewInternalf("decode ParadeDB visibility execution: %v", err)
	}
	return &store.InternalGetWorkflowExecutionResponse{Execution: info}, nil
}

func (*VisibilityStore) AddSearchAttributes(context.Context, *manager.AddSearchAttributesRequest) error {
	return nil
}

func (s *VisibilityStore) encodeRequestAttributes(attrs *commonpb.SearchAttributes) (encodedSearchAttributes, error) {
	types, err := s.searchAttributeTypes()
	if err != nil {
		return encodedSearchAttributes{}, err
	}
	encoded, err := encodeSearchAttributes(attrs, types)
	for _, name := range encoded.skipped {
		s.logger.Warn("Skipping unknown or non-scalar search attribute while writing visibility record", tag.String("search-attribute", name))
	}
	return encoded, err
}

func visibilityWriteError(action string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", action, err)
	}
	return serviceerror.NewUnavailablef("%s: %v", action, err)
}
