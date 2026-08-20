package callbacks

import (
	chasmcallbacks "github.com/superdurable/temporal-server/chasm/lib/callback"
)

var (
	RequestCounter          = chasmcallbacks.RequestCounter
	RequestLatencyHistogram = chasmcallbacks.RequestLatencyHistogram
)
