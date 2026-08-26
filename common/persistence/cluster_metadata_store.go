package persistence

import (
	"context"
	"errors"

	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"go.temporal.io/api/serviceerror"
)

var (
	// ErrInvalidMembershipExpiry is used when upserting new cluster membership with an invalid duration
	ErrInvalidMembershipExpiry = errors.New("membershipExpiry duration should be atleast 1 second")

	// ErrIncompleteMembershipUpsert is used when upserting new cluster membership with missing fields
	ErrIncompleteMembershipUpsert = errors.New("membership upserts require all fields")
)

type (
	// clusterMetadataManagerImpl implements MetadataManager based on MetadataStore and Serializer
	clusterMetadataManagerImpl struct {
		serializer         serialization.Serializer
		persistence        ClusterMetadataStore
		currentClusterName string
		logger             log.Logger
	}
)

var _ ClusterMetadataManager = (*clusterMetadataManagerImpl)(nil)

// NewClusterMetadataManagerImpl returns new ClusterMetadataManager
func NewClusterMetadataManagerImpl(
	persistence ClusterMetadataStore,
	serializer serialization.Serializer,
	currentClusterName string,
	logger log.Logger,
) ClusterMetadataManager {
	return &clusterMetadataManagerImpl{
		serializer:         serializer,
		persistence:        persistence,
		currentClusterName: currentClusterName,
		logger:             logger,
	}
}

func (m *clusterMetadataManagerImpl) GetName() string {
	return m.persistence.GetName()
}

func (m *clusterMetadataManagerImpl) Close() {
	m.persistence.Close()
}

func (m *clusterMetadataManagerImpl) GetClusterMembers(
	ctx context.Context,
	request *GetClusterMembersRequest,
) (*GetClusterMembersResponse, error) {
	return m.persistence.GetClusterMembers(ctx, request)
}

func (m *clusterMetadataManagerImpl) UpsertClusterMembership(
	ctx context.Context,
	request *UpsertClusterMembershipRequest,
) error {
	if request.RecordExpiry.Seconds() < 1 {
		return ErrInvalidMembershipExpiry
	}
	if request.Role == All {
		return ErrIncompleteMembershipUpsert
	}
	if request.RPCAddress == nil {
		return ErrIncompleteMembershipUpsert
	}
	if request.RPCPort == 0 {
		return ErrIncompleteMembershipUpsert
	}
	if request.SessionStart.IsZero() {
		return ErrIncompleteMembershipUpsert
	}

	return m.persistence.UpsertClusterMembership(ctx, request)
}

func (m *clusterMetadataManagerImpl) PruneClusterMembership(
	ctx context.Context,
	request *PruneClusterMembershipRequest,
) error {
	return m.persistence.PruneClusterMembership(ctx, request)
}

func (m *clusterMetadataManagerImpl) ListClusterMetadata(
	ctx context.Context,
	request *ListClusterMetadataRequest,
) (*ListClusterMetadataResponse, error) {
	resp, err := m.persistence.ListClusterMetadata(ctx, &InternalListClusterMetadataRequest{
		PageSize:      request.PageSize,
		NextPageToken: request.NextPageToken,
	})
	if err != nil {
		return nil, err
	}

	clusterMetadata := make([]*GetClusterMetadataResponse, 0, len(resp.ClusterMetadata))
	for _, cm := range resp.ClusterMetadata {
		res, err := m.convertInternalGetClusterMetadataResponse(cm)
		if err != nil {
			return nil, err
		}
		clusterMetadata = append(clusterMetadata, res)
	}
	return &ListClusterMetadataResponse{ClusterMetadata: clusterMetadata, NextPageToken: resp.NextPageToken}, nil
}

func (m *clusterMetadataManagerImpl) GetCurrentClusterMetadata(
	ctx context.Context,
) (*GetClusterMetadataResponse, error) {
	resp, err := m.persistence.GetClusterMetadata(ctx, &InternalGetClusterMetadataRequest{ClusterName: m.currentClusterName})
	if err != nil {
		return nil, err
	}

	mcm, err := m.serializer.DeserializeClusterMetadata(resp.ClusterMetadata)
	if err != nil {
		return nil, err
	}
	return &GetClusterMetadataResponse{ClusterMetadata: mcm, Version: resp.Version}, nil
}

func (m *clusterMetadataManagerImpl) GetClusterMetadata(
	ctx context.Context,
	request *GetClusterMetadataRequest,
) (*GetClusterMetadataResponse, error) {
	resp, err := m.persistence.GetClusterMetadata(ctx, &InternalGetClusterMetadataRequest{ClusterName: request.ClusterName})
	if err != nil {
		return nil, err
	}

	mcm, err := m.serializer.DeserializeClusterMetadata(resp.ClusterMetadata)
	if err != nil {
		return nil, err
	}
	return &GetClusterMetadataResponse{ClusterMetadata: mcm, Version: resp.Version}, nil
}

func (m *clusterMetadataManagerImpl) SaveClusterMetadata(
	ctx context.Context,
	request *SaveClusterMetadataRequest,
) (bool, error) {
	mcm, err := m.serializer.SerializeClusterMetadata(request.ClusterMetadata)
	if err != nil {
		return false, err
	}

	oldClusterMetadata, err := m.GetClusterMetadata(ctx, &GetClusterMetadataRequest{ClusterName: request.GetClusterName()})
	if _, isNotFound := err.(*serviceerror.NotFound); isNotFound {
		return m.persistence.SaveClusterMetadata(ctx, &InternalSaveClusterMetadataRequest{
			ClusterName:     request.ClusterName,
			ClusterMetadata: mcm,
			Version:         request.Version,
		})
	}
	if err != nil {
		return false, err
	}
	if immutableFieldsChanged(oldClusterMetadata.ClusterMetadata, request.ClusterMetadata) {
		return false, nil
	}

	return m.persistence.SaveClusterMetadata(ctx, &InternalSaveClusterMetadataRequest{
		ClusterName:     request.ClusterName,
		ClusterMetadata: mcm,
		Version:         request.Version,
	})
}

func (m *clusterMetadataManagerImpl) DeleteClusterMetadata(
	ctx context.Context,
	request *DeleteClusterMetadataRequest,
) error {
	if request.ClusterName == m.currentClusterName {
		return serviceerror.NewInvalidArgument("Cannot delete current cluster metadata")
	}

	return m.persistence.DeleteClusterMetadata(ctx, &InternalDeleteClusterMetadataRequest{ClusterName: request.ClusterName})
}

func (m *clusterMetadataManagerImpl) convertInternalGetClusterMetadataResponse(
	resp *InternalGetClusterMetadataResponse,
) (*GetClusterMetadataResponse, error) {
	mcm, err := m.serializer.DeserializeClusterMetadata(resp.ClusterMetadata)
	if err != nil {
		return nil, err
	}

	return &GetClusterMetadataResponse{
		ClusterMetadata: mcm,
		Version:         resp.Version,
	}, nil
}

// ValidateHistoryShardCountTransition reports whether a cluster's history shard count may move from
// old to cur.
//
// A workflow's shard is hash%N+1 (see common.WorkflowIDToHistoryShard), so when N grows to k*N every
// old shard s splits deterministically into {s, s+N, ..., s+(k-1)N}: no workflow can land outside
// that set, which is what makes the change tractable. When k is not an integer the mapping is
// arbitrary, and common.MapShardID / common.VerifyShardIDMapping — which replication relies on to
// pair shards across clusters — panic outright on counts that do not divide evenly.
//
// Shrinking is rejected: merging two shards would collide their independent task ID sequences, which
// needs task rewriting rather than just a different modulus.
func ValidateHistoryShardCountTransition(old int32, cur int32) error {
	switch {
	case old == cur:
		return nil
	case old == 0 || cur == 0:
		return serviceerror.NewInvalidArgumentf(
			"history shard count must be non-zero, got %v -> %v", old, cur)
	case cur < old:
		return serviceerror.NewInvalidArgumentf(
			"history shard count cannot be decreased (%v -> %v)", old, cur)
	case cur%old != 0:
		return serviceerror.NewInvalidArgumentf(
			"history shard count can only be increased to an integer multiple of the current count (%v -> %v)",
			old, cur)
	default:
		return nil
	}
}

// immutableFieldsChanged returns true if any of immutable fields changed.
func immutableFieldsChanged(old *persistencespb.ClusterMetadata, cur *persistencespb.ClusterMetadata) bool {
	if (old.ClusterName != "" && old.ClusterName != cur.ClusterName) ||
		(old.ClusterId != "" && old.ClusterId != cur.ClusterId) ||
		(old.HistoryShardCount != 0 && ValidateHistoryShardCountTransition(old.HistoryShardCount, cur.HistoryShardCount) != nil) ||
		(old.IsGlobalNamespaceEnabled && !cur.IsGlobalNamespaceEnabled) {
		return true
	}
	if old.IsGlobalNamespaceEnabled {
		if (old.FailoverVersionIncrement != 0 && old.FailoverVersionIncrement != cur.FailoverVersionIncrement) ||
			(old.InitialFailoverVersion != 0 && old.InitialFailoverVersion != cur.InitialFailoverVersion) {
			return true
		}
	}
	return false
}
