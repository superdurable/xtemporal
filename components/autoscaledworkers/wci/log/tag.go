package log

import "github.com/superdurable/xtemporal/common/log/tag"

var ComponentWorkerControllerWorkers = componentTag("worker-controller-workers")

// Component returns tag for Component
func componentTag(component string) tag.ZapTag {
	return tag.NewStringTag("component", component)
}
