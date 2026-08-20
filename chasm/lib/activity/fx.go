package activity

import (
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/chasm/lib/activity/gen/activitypb/v1"
	"github.com/superdurable/temporal-server/common/resource"
	"go.uber.org/fx"
)

var HistoryModule = fx.Module(
	"activity-history",
	fx.Provide(
		ConfigProvider,
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
