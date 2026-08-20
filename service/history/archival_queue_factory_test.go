package history

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/common/clock"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/primitives/timestamp"
	"github.com/superdurable/temporal-server/service/history/shard"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"github.com/superdurable/temporal-server/service/history/tests"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestArchivalQueueFactory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	metricsHandler := metrics.NewMockHandler(ctrl)
	metricsHandler.EXPECT().WithTags(gomock.Any()).DoAndReturn(
		func(tags ...metrics.Tag) metrics.Handler {
			require.Len(t, tags, 1)
			assert.Equal(t, metrics.OperationTagName, tags[0].Key)
			assert.Equal(t, "ArchivalQueueProcessor", tags[0].Value)
			return metricsHandler
		},
	).Times(1)

	mockShard := shard.NewTestContext(
		ctrl,
		&persistencespb.ShardInfo{
			ShardId: 0,
			RangeId: 1,
			QueueStates: map[int32]*persistencespb.QueueState{
				int32(tasks.CategoryIDArchival): {
					ReaderStates: nil,
					ExclusiveReaderHighWatermark: &persistencespb.TaskKey{
						FireTime: timestamp.TimeNowPtrUtc(),
					},
				},
			},
		},
		tests.NewDynamicConfig(),
	)
	mockShard.Resource.ClusterMetadata.EXPECT().GetCurrentClusterName().Return(cluster.TestCurrentClusterName).AnyTimes()

	queueFactory := NewArchivalQueueFactory(ArchivalQueueFactoryParams{
		QueueFactoryBaseParams: QueueFactoryBaseParams{
			NamespaceRegistry: mockShard.Resource.GetNamespaceRegistry(),
			ClusterMetadata:   mockShard.Resource.GetClusterMetadata(),
			Config:            tests.NewDynamicConfig(),
			TimeSource:        clock.NewEventTimeSource(),
			MetricsHandler:    metricsHandler,
			Logger:            log.NewNoopLogger(),
			TracerProvider:    noop.NewTracerProvider(),
		},
	})
	queue := queueFactory.CreateQueue(mockShard)

	require.NotNil(t, queue)
	assert.Equal(t, tasks.CategoryArchival, queue.Category())
}
