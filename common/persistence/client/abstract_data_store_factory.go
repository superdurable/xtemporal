package client

import (
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"github.com/superdurable/temporal-server/common/resolver"
)

type (

	// AbstractDataStoreFactory creates a DataStoreFactory, can be used to implement custom datastore support outside
	// of the Temporal core.
	AbstractDataStoreFactory interface {
		NewFactory(
			cfg config.CustomDatastoreConfig,
			r resolver.ServiceResolver,
			clusterName string,
			logger log.Logger,
			metricsHandler metrics.Handler,
			serializer serialization.Serializer,
		) persistence.DataStoreFactory
	}
)
