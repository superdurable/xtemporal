package matching

import "github.com/superdurable/xtemporal/common/tqid"

type RoutingClient interface {
	Route(p tqid.Partition) (string, error)
}
