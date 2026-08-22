package cluster

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCurrentShardCount_FollowsMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	metadata := NewMockMetadata(ctrl)

	shardCount := CurrentShardCount(metadata, 4)

	metadata.EXPECT().GetCurrentShardCount().Return(ShardCount{Count: 4, Version: 1})
	require.Equal(t, int32(4), shardCount())

	metadata.EXPECT().GetCurrentShardCount().Return(ShardCount{Count: 8, Version: 2})
	require.Equal(t, int32(8), shardCount(), "a resize must be visible without reconstructing the supplier")
}

func TestCurrentShardCount_FallsBackBeforeMetadataIsLoaded(t *testing.T) {
	ctrl := gomock.NewController(t)
	metadata := NewMockMetadata(ctrl)
	metadata.EXPECT().GetCurrentShardCount().Return(ShardCount{})

	require.Equal(t, int32(4), CurrentShardCount(metadata, 4)())
}

func TestStaticShardCount(t *testing.T) {
	require.Equal(t, int32(512), StaticShardCount(512)())
}

func TestLazyMetadata_ShardCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	metadata := NewMockMetadata(ctrl)

	lazy := NewLazyMetadata()
	shardCount := lazy.ShardCount(4)

	// Handed out before Set, which is the whole point of the holder.
	require.Equal(t, int32(4), shardCount())

	lazy.Set(metadata)
	metadata.EXPECT().GetCurrentShardCount().Return(ShardCount{Count: 8, Version: 1})
	require.Equal(t, int32(8), shardCount())
}
