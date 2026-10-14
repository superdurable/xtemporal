package queues

import (
	"github.com/superdurable/xtemporal/service/history/tasks"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination queue_mock.go

type (
	Queue interface {
		Category() tasks.Category
		NotifyNewTasks(tasks []tasks.Task)
		FailoverNamespace(namespaceID string)
		Start()
		Stop()
	}
)
