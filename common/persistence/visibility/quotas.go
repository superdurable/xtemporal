package visibility

import (
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/headers"
	"github.com/superdurable/xtemporal/common/quotas"
)

const (
	// OperatorPriority is used to give precedence to calls coming from web UI or tctl
	OperatorPriority = 0
)

var (
	PrioritiesOrdered = []int{OperatorPriority, 1}
)

func newPriorityRateLimiter(
	maxQPS dynamicconfig.IntPropertyFn,
	operatorRPSRatio dynamicconfig.FloatPropertyFn,
) quotas.RequestRateLimiter {
	rateLimiters := make(map[int]quotas.RequestRateLimiter)
	for priority := range PrioritiesOrdered {
		if priority == OperatorPriority {
			rateLimiters[priority] = quotas.NewRequestRateLimiterAdapter(quotas.NewDefaultOutgoingRateLimiter(operatorRateFn(maxQPS, operatorRPSRatio)))
		} else {
			rateLimiters[priority] = quotas.NewRequestRateLimiterAdapter(quotas.NewDefaultOutgoingRateLimiter(rateFn(maxQPS)))
		}
	}
	return quotas.NewPriorityRateLimiter(func(req quotas.Request) int {
		if req.CallerType == headers.CallerTypeOperator {
			return OperatorPriority
		}
		// default to lowest priority
		return PrioritiesOrdered[len(PrioritiesOrdered)-1]
	}, rateLimiters)
}

func rateFn(maxQPS dynamicconfig.IntPropertyFn) quotas.RateFn {
	return func() float64 {
		return float64(maxQPS())
	}
}

func operatorRateFn(maxQPS dynamicconfig.IntPropertyFn, operatorRPSRatio dynamicconfig.FloatPropertyFn) quotas.RateFn {
	return func() float64 {
		return float64(maxQPS()) * operatorRPSRatio()
	}
}
