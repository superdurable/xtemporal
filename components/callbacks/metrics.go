package callbacks

import (
	chasmcallbacks "github.com/superdurable/xtemporal/chasm/lib/callback"
)

var (
	RequestCounter          = chasmcallbacks.RequestCounter
	RequestLatencyHistogram = chasmcallbacks.RequestLatencyHistogram
)
