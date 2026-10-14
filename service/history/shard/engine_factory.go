//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination engine_factory_mock.go

package shard

import historyi "github.com/superdurable/xtemporal/service/history/interfaces"

type (
	// EngineFactory is used to create an instance of sharded history engine
	EngineFactory interface {
		CreateEngine(context historyi.ShardContext) historyi.Engine
	}
)
