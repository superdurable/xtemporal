package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/uber-go/tally/v4"
)

type prefixRecordingHandler struct {
	names      []string
	batchNames []string
	closed     bool
}

func (h *prefixRecordingHandler) WithTags(...Tag) Handler {
	return h
}

func (h *prefixRecordingHandler) Counter(name string) CounterIface {
	h.names = append(h.names, name)
	return NoopCounterMetricFunc
}

func (h *prefixRecordingHandler) Gauge(name string) GaugeIface {
	h.names = append(h.names, name)
	return NoopGaugeMetricFunc
}

func (h *prefixRecordingHandler) Timer(name string) TimerIface {
	h.names = append(h.names, name)
	return NoopTimerMetricFunc
}

func (h *prefixRecordingHandler) Histogram(name string, _ MetricUnit) HistogramIface {
	h.names = append(h.names, name)
	return NoopHistogramMetricFunc
}

func (*prefixRecordingHandler) Stop(log.Logger) {}

func (h *prefixRecordingHandler) StartBatch(name string) BatchHandler {
	h.batchNames = append(h.batchNames, name)
	return h
}

func (h *prefixRecordingHandler) Close() error {
	h.closed = true
	return nil
}

func TestPrefixedHandler(t *testing.T) {
	t.Parallel()

	recordingHandler := &prefixRecordingHandler{}
	handler := newPrefixedHandler(recordingHandler)

	handler.Counter("counter").Record(1)
	handler.Gauge("gauge").Record(1)
	handler.Timer("timer").Record(time.Second)
	handler.Histogram("histogram", Bytes).Record(1)
	handler.WithTags(StringTag("tag", "value")).Counter("tagged").Record(1)
	handler.Counter("dex_engine_already_prefixed").Record(1)

	batch := handler.StartBatch("batch")
	batch.Counter("batched").Record(1)
	require.NoError(t, batch.Close())

	require.Equal(t, []string{
		"dex_engine_counter",
		"dex_engine_gauge",
		"dex_engine_timer",
		"dex_engine_histogram",
		"dex_engine_tagged",
		"dex_engine_already_prefixed",
		"dex_engine_batched",
	}, recordingHandler.names)
	require.Equal(t, []string{"dex_engine_batch"}, recordingHandler.batchNames)
	require.True(t, recordingHandler.closed)
}

func TestPrefixedTallyHandler(t *testing.T) {
	t.Parallel()

	scope := tally.NewTestScope("test", map[string]string{})
	handler := newPrefixedHandlerWithPrefix(
		NewTallyMetricsHandler(defaultConfig, scope),
		dexEngineMetricPrefixWithClientPrefix("examples_"),
	)
	handler.Counter("counter").Record(1)

	require.Contains(t, scope.Snapshot().Counters(), "test.examples_dex_engine_counter+")
}

func TestDexEngineMetricPrefixWithClientPrefix(t *testing.T) {
	t.Parallel()

	require.Equal(t, "dex_engine_", dexEngineMetricPrefixWithClientPrefix(""))
	require.Equal(t, "examples_dex_engine_", dexEngineMetricPrefixWithClientPrefix("examples"))
	require.Equal(t, "examples_dex_engine_", dexEngineMetricPrefixWithClientPrefix("examples_"))
}
