package cluster

import "sync/atomic"

// ShardCountFn reports the cluster's current history shard count.
//
// The count is a function rather than a value because a cluster can grow its
// shard count while running. Anything that captures the count once keeps
// routing against the pre-resize shard map for the rest of the process's life,
// which sends requests to a shard that no longer owns the workflow.
type ShardCountFn func() int32

// CurrentShardCount reads the live count from cluster metadata.
//
// bootstrapCount covers the window before metadata has been loaded from
// persistence, since several components are constructed ahead of that load.
func CurrentShardCount(metadata Metadata, bootstrapCount int32) ShardCountFn {
	return func() int32 {
		if count := metadata.GetCurrentShardCount().Count; count > 0 {
			return count
		}
		return bootstrapCount
	}
}

// StaticShardCount is for callers that are handed the shard count rather than
// discovering it, such as CLI tools and tests.
func StaticShardCount(count int32) ShardCountFn {
	return func() int32 { return count }
}

// LazyMetadata hands out a ShardCountFn before Metadata itself can be built.
//
// The history service config needs the shard count, the persistence client
// factory is configured from that config, and building Metadata needs the
// persistence client factory. Depending on Metadata directly would close that
// loop, so the holder is created empty and filled in once Metadata exists.
type LazyMetadata struct {
	*atomic.Value // value type is Metadata
}

func NewLazyMetadata() LazyMetadata {
	return LazyMetadata{Value: &atomic.Value{}}
}

func (l LazyMetadata) Set(metadata Metadata) {
	l.Store(metadata)
}

// ShardCount reports the live count, falling back to bootstrapCount until
// Metadata is both set and loaded from persistence.
func (l LazyMetadata) ShardCount(bootstrapCount int32) ShardCountFn {
	return func() int32 {
		if metadata, ok := l.Load().(Metadata); ok {
			if count := metadata.GetCurrentShardCount().Count; count > 0 {
				return count
			}
		}
		return bootstrapCount
	}
}
