package workflow

import (
	"github.com/superdurable/xtemporal/api/historyservice/v1"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/chasm/lib/nexusoperation"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"chasm.lib.workflow",
	fx.Provide(NewConfig),
	fx.Provide(NewRegistry),
	fx.Provide(newLibrary),
	fx.Invoke(func(
		chasmRegistry *chasm.Registry,
		library *library,
		config *nexusoperation.Config,
	) error {
		if err := library.registry.Register(
			newNexusLibrary(config, chasmRegistry.NexusEndpointProcessor),
		); err != nil {
			return err
		}
		return chasmRegistry.Register(library)
	}),
)

// HistoryHandlerModule wires the workflow library's Nexus handler to the
// history service. Only include this in services that provide
// historyservice.HistoryServiceServer (the history service).
var HistoryHandlerModule = fx.Invoke(func(library *library, historyHandler historyservice.HistoryServiceServer) {
	library.workflowServiceNexusHandler.setHistoryHandler(historyHandler)
})
