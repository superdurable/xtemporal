package client

import (
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	"github.com/superdurable/xtemporal/common/resolver"
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
