package deadlock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/clock"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/goro"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/metrics/metricstest"
	"github.com/superdurable/xtemporal/common/pingable"
)

type blockingPingable struct{ done chan struct{} }

func (b *blockingPingable) GetPingChecks() []pingable.Check {
	return []pingable.Check{{
		Name:    "test",
		Timeout: 10 * time.Millisecond,
		Ping: func() []pingable.Pingable {
			<-b.done
			return nil
		},
	}}
}

func TestCurrentCounterAndGauge(t *testing.T) {
	mh := metricstest.NewCaptureHandler()
	dd := NewDeadlockDetector(params{
		Logger:         log.NewNoopLogger(),
		Collection:     dynamicconfig.NewNoopCollection(),
		MetricsHandler: mh,
	})

	lc := &loopContext{
		dd:   dd,
		p:    goro.NewAdaptivePool(clock.NewRealTimeSource(), 0, 1, 10*time.Millisecond, 10),
		root: nil,
	}
	defer lc.p.Stop()

	b := &blockingPingable{done: make(chan struct{})}
	check := b.GetPingChecks()[0]

	capture := mh.StartCapture()
	go lc.check(context.Background(), check)

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		require.Equal(collect, int64(1), dd.CurrentSuspected())

		snapshot := capture.Snapshot()
		current := snapshot[metrics.DDCurrentSuspectedDeadlocks.Name()]
		counter := snapshot[metrics.DDSuspectedDeadlocks.Name()]
		require.Len(collect, current, 1)
		require.Equal(collect, 1.0, current[0].Value)
		require.Len(collect, counter, 1)
		require.Equal(collect, int64(1), counter[0].Value)
	}, 2*time.Second, time.Millisecond)

	close(b.done)

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		require.Equal(collect, int64(0), dd.CurrentSuspected())

		snapshot := capture.Snapshot()
		current := snapshot[metrics.DDCurrentSuspectedDeadlocks.Name()]
		counter := snapshot[metrics.DDSuspectedDeadlocks.Name()]
		require.Len(collect, current, 2)
		require.Equal(collect, 0.0, current[1].Value)
		require.Len(collect, counter, 1)
	}, 2*time.Second, time.Millisecond)
}
