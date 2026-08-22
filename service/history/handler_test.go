package history

import (
	"context"
	"errors"
	"github.com/superdurable/temporal-server/common/cluster"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superdurable/temporal-server/api/historyservice/v1"
	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/membership"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/serviceerror"
	"github.com/superdurable/temporal-server/service/history/configs"
	"github.com/superdurable/temporal-server/service/history/shard"
	"github.com/superdurable/temporal-server/service/history/tests"
	"go.uber.org/mock/gomock"
)

func TestDescribeHistoryHost(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	controller := shard.NewMockController(ctrl)
	namespaceRegistry := namespace.NewMockRegistry(ctrl)
	hostInfoProvider := membership.NewMockHostInfoProvider(ctrl)
	h := Handler{
		config: &configs.Config{
			NumberOfShards: cluster.StaticShardCount(10),
		},
		metricsHandler:    metrics.NoopMetricsHandler,
		logger:            log.NewNoopLogger(),
		controller:        controller,
		namespaceRegistry: namespaceRegistry,
		hostInfoProvider:  hostInfoProvider,
	}

	mockShard1 := shard.NewTestContext(
		ctrl,
		&persistencespb.ShardInfo{
			ShardId: 1,
			RangeId: 1,
		},
		tests.NewDynamicConfig(),
	)
	controller.EXPECT().GetShardByID(int32(1)).Return(mockShard1, serviceerror.NewShardOwnershipLost("", ""))

	_, err := h.DescribeHistoryHost(context.Background(), &historyservice.DescribeHistoryHostRequest{
		ShardId: 1,
	})
	assert.Error(t, err)
	var sol *serviceerror.ShardOwnershipLost
	assert.True(t, errors.As(err, &sol))

	mockShard2 := shard.NewTestContext(
		ctrl,
		&persistencespb.ShardInfo{
			ShardId: 2,
			RangeId: 1,
		},
		tests.NewDynamicConfig(),
	)
	controller.EXPECT().GetShardByID(int32(2)).Return(mockShard2, nil)
	controller.EXPECT().ShardIDs().Return([]int32{2})
	namespaceRegistry.EXPECT().GetRegistrySize().Return(int64(0), int64(0))
	hostInfoProvider.EXPECT().HostInfo().Return(membership.NewHostInfoFromAddress("0.0.0.0"))
	_, err = h.DescribeHistoryHost(context.Background(), &historyservice.DescribeHistoryHostRequest{
		ShardId: 2,
	})
	assert.NoError(t, err)
}
