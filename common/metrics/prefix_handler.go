package metrics

import (
	"strings"

	"github.com/superdurable/temporal-server/common/log"
)

const dexEngineMetricPrefix = "dex_engine_"

type prefixedHandler struct {
	handler      Handler
	metricPrefix string
}

type prefixedBatchHandler struct {
	Handler
	batchHandler BatchHandler
}

func newPrefixedHandler(handler Handler) Handler {
	return newPrefixedHandlerWithPrefix(handler, dexEngineMetricPrefix)
}

func newPrefixedHandlerWithPrefix(handler Handler, metricPrefix string) Handler {
	return &prefixedHandler{
		handler:      handler,
		metricPrefix: metricPrefix,
	}
}

func prefixMetricName(name string) string {
	return prefixMetricNameWithPrefix(dexEngineMetricPrefix, name)
}

func prefixMetricNameWithPrefix(metricPrefix string, name string) string {
	if strings.HasPrefix(name, metricPrefix) {
		return name
	}
	return metricPrefix + name
}

func dexEngineMetricPrefixWithClientPrefix(clientPrefix string) string {
	if clientPrefix == "" || strings.HasSuffix(clientPrefix, "_") {
		return clientPrefix + dexEngineMetricPrefix
	}
	return clientPrefix + "_" + dexEngineMetricPrefix
}

func (h *prefixedHandler) WithTags(tags ...Tag) Handler {
	return newPrefixedHandlerWithPrefix(h.handler.WithTags(tags...), h.metricPrefix)
}

func (h *prefixedHandler) Counter(name string) CounterIface {
	return h.handler.Counter(prefixMetricNameWithPrefix(h.metricPrefix, name))
}

func (h *prefixedHandler) Gauge(name string) GaugeIface {
	return h.handler.Gauge(prefixMetricNameWithPrefix(h.metricPrefix, name))
}

func (h *prefixedHandler) Timer(name string) TimerIface {
	return h.handler.Timer(prefixMetricNameWithPrefix(h.metricPrefix, name))
}

func (h *prefixedHandler) Histogram(name string, unit MetricUnit) HistogramIface {
	return h.handler.Histogram(prefixMetricNameWithPrefix(h.metricPrefix, name), unit)
}

func (h *prefixedHandler) Stop(logger log.Logger) {
	h.handler.Stop(logger)
}

func (h *prefixedHandler) StartBatch(name string) BatchHandler {
	batchHandler := h.handler.StartBatch(prefixMetricNameWithPrefix(h.metricPrefix, name))
	return &prefixedBatchHandler{
		Handler:      newPrefixedHandlerWithPrefix(batchHandler, h.metricPrefix),
		batchHandler: batchHandler,
	}
}

func (h *prefixedBatchHandler) Close() error {
	return h.batchHandler.Close()
}
