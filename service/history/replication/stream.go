package replication

import (
	"context"
	"errors"
	"time"

	"github.com/superdurable/xtemporal/api/adminservice/v1"
	"github.com/superdurable/xtemporal/common/backoff"
	"github.com/superdurable/xtemporal/common/channel"
	"github.com/superdurable/xtemporal/common/cluster"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/persistence"
	serviceerrors "github.com/superdurable/xtemporal/common/serviceerror"
	"github.com/superdurable/xtemporal/service/history/configs"
	"go.temporal.io/api/serviceerror"
)

type (
	Stream          BiDirectionStream[*adminservice.StreamWorkflowReplicationMessagesRequest, *adminservice.StreamWorkflowReplicationMessagesResponse]
	ClusterShardKey struct {
		ClusterID int32
		ShardID   int32
	}
	ClusterShardKeyPair struct {
		Client ClusterShardKey
		Server ClusterShardKey
	}
)

func ClusterIDToClusterNameShardCount(
	allClusterInfo map[string]cluster.ClusterInformation,
	clusterID int32,
) (string, int32, error) {
	for clusterName, clusterInfo := range allClusterInfo {
		if int32(clusterInfo.InitialFailoverVersion) == clusterID {
			return clusterName, clusterInfo.ShardCount, nil
		}
	}
	return "", 0, serviceerror.NewInternalf("unknown cluster ID: %v", clusterID)
}

func WrapEventLoop(
	ctx context.Context,
	originalEventLoop func() error,
	streamStopper func(),
	logger log.Logger,
	metricsHandler metrics.Handler,
	fromClusterKey ClusterShardKey,
	toClusterKey ClusterShardKey,
	dc *configs.Config,
) {
	defer streamStopper()

	streamRetryPolicy := backoff.NewExponentialRetryPolicy(500 * time.Millisecond).
		WithMaximumAttempts(dc.ReplicationStreamEventLoopRetryMaxAttempts()).
		WithMaximumInterval(time.Second * 2)
	ops := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := originalEventLoop()

		if err != nil {
			var streamError *StreamError
			if errors.As(err, &streamError) {
				metrics.ReplicationStreamError.With(metricsHandler).Record(
					int64(1),
					metrics.ServiceErrorTypeTag(streamError.cause),
					metrics.FromClusterIDTag(fromClusterKey.ClusterID),
					metrics.ToClusterIDTag(toClusterKey.ClusterID),
				)
				logger.Warn("ReplicationStreamError", tag.Error(err))
			} else {
				metrics.ReplicationServiceError.With(metricsHandler).Record(
					int64(1),
					metrics.ServiceErrorTypeTag(err),
					metrics.FromClusterIDTag(fromClusterKey.ClusterID),
					metrics.ToClusterIDTag(toClusterKey.ClusterID),
				)
				logger.Error("ReplicationServiceError", tag.Error(err))
			}
			return err
		}
		// shutdown case
		return nil
	}
	_ = backoff.ThrottleRetry(ops, streamRetryPolicy, isRetryableError)
}

func livenessMonitor(
	signalChan <-chan struct{},
	timeoutFn dynamicconfig.DurationPropertyFn,
	timeoutMultiplier dynamicconfig.IntPropertyFn,
	shutdownChan channel.ShutdownOnce,
	stopStream func(),
	logger log.Logger,
) {
	select {
	case <-shutdownChan.Channel():
		return
	case <-signalChan:
		// Wait for the first message into stream before monitoring.
	}

	heartbeatTimeout := time.NewTimer(timeoutFn() * time.Duration(timeoutMultiplier()))
	defer heartbeatTimeout.Stop()

	for !shutdownChan.IsShutdown() {
		select {
		case <-shutdownChan.Channel():
			return
		case <-heartbeatTimeout.C:
			select {
			case <-signalChan:
				if !heartbeatTimeout.Stop() {
					select {
					case <-heartbeatTimeout.C:
					default:
					}
				}
				heartbeatTimeout.Reset(timeoutFn() * time.Duration(timeoutMultiplier()))
				continue
			default:
				logger.Warn("No liveness signal received. Stop the replication stream.")
				stopStream()
				return
			}
		}
	}
}

func isRetryableError(err error) bool {
	var streamError *StreamError
	var shardOwnershipLostError *persistence.ShardOwnershipLostError
	var shardOwnershipLost *serviceerrors.ShardOwnershipLost
	switch {
	case errors.As(err, &shardOwnershipLostError):
		return false
	case errors.As(err, &shardOwnershipLost):
		return false
	case errors.As(err, &streamError):
		return false
	case errors.Is(err, context.Canceled):
		return false
	default:
		return true
	}
}
