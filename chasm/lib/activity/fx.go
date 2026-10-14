package activity

import (
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/chasm/lib/activity/gen/activitypb/v1"
	"github.com/superdurable/xtemporal/common/resource"
	"go.uber.org/fx"
)

var HistoryModule = fx.Module(
	"activity-history",
	fx.Provide(
		ConfigProvider,
		linkValidatorProvider,
		newActivityDispatchTaskHandler,
		newScheduleToStartTimeoutTaskHandler,
		newScheduleToCloseTimeoutTaskHandler,
		newStartToCloseTimeoutTaskHandler,
		newHeartbeatTimeoutTaskHandler,
		newHandler,
		newLibrary,
	),
	fx.Invoke(func(l *library, registry *chasm.Registry) error {
		return registry.Register(l)
	}),
)

var FrontendModule = fx.Module(
	"activity-frontend",
	fx.Provide(ConfigProvider),
	fx.Provide(linkValidatorProvider),
	fx.Provide(activitypb.NewActivityServiceLayeredClient),
	fx.Provide(NewFrontendHandler),
	fx.Provide(resource.SearchAttributeValidatorProvider),
	fx.Provide(newComponentOnlyLibrary),
	fx.Invoke(func(l *componentOnlyLibrary, registry *chasm.Registry) error {
		// Frontend needs to register the component in order to serialize ComponentRefs, but doesn't
		// need task handlers.
		return registry.Register(l)
	}),
)
