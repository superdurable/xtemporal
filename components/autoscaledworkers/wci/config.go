package wci

import (
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/components/autoscaledworkers/wci/client"
	"github.com/superdurable/xtemporal/service/worker"
)

type (
	// Config contains all the service config for worker
	Config struct {
		worker.Config

		PerNamespaceMaxWorkerControllerInstances            dynamicconfig.TypedPropertyFnWithNamespaceFilter[int]
		PerNamespaceWorkerControllerInstanceWorkflowVersion dynamicconfig.TypedPropertyFnWithNamespaceFilter[int]
	}
)

// NewConfig builds the new Config for worker service
func NewConfig(
	dc *dynamicconfig.Collection,
	persistenceConfig *config.Persistence,
) *Config {
	return &Config{
		Config: *worker.NewConfig(dc, persistenceConfig),

		PerNamespaceMaxWorkerControllerInstances:            client.WorkerControllerMaxInstances.Get(dc),
		PerNamespaceWorkerControllerInstanceWorkflowVersion: client.WorkerControllerInstanceWorkflowVersion.Get(dc),
	}
}
