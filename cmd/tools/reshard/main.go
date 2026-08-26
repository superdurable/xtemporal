// Command temporal-reshard relocates pending history tasks after the history shard count has been
// increased.
//
// It reads the target count from persistence.numHistoryShards in the server config and the current
// count from cluster metadata, so it needs no arguments beyond the config the servers themselves
// use. Run it with the cluster stopped, after raising numHistoryShards and before starting the
// servers again:
//
//	temporal-reshard --root . --env production
package main

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/persistence"
	persistenceClient "github.com/superdurable/temporal-server/common/persistence/client"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"github.com/superdurable/temporal-server/common/persistence/sql"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin"
	_ "github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin/mysql"      // needed to load mysql plugin
	_ "github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin/postgresql" // needed to load postgresql plugin
	_ "github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin/sqlite"     // needed to load sqlite plugin
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/common/resolver"
	"github.com/superdurable/temporal-server/common/telemetry"
	"github.com/superdurable/temporal-server/tools/reshard"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "temporal-reshard",
		Usage: "Relocate pending history tasks after increasing the history shard count",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "root",
				Value: ".",
				Usage: "root directory of the config files",
			},
			&cli.StringFlag{
				Name:  "config",
				Value: "config",
				Usage: "config directory within root",
			},
			&cli.StringFlag{
				Name:  "env",
				Value: "development",
				Usage: "runtime environment",
			},
			&cli.StringFlag{
				Name:  "zone",
				Usage: "availability zone",
			},
			&cli.IntFlag{
				Name:  "batch-size",
				Value: 1000,
				Usage: "number of tasks to read per query",
			},
			&cli.IntFlag{
				Name:  "concurrency",
				Value: 4,
				Usage: "number of shards to relocate at once, bounded in practice by the connection pool",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "scan and report what would move, without changing anything",
			},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(c *cli.Context) error {
	logger := log.NewCLILogger()

	cfg, err := config.Load(
		config.WithEnv(c.String("env")),
		config.WithConfigDir(path.Join(c.String("root"), c.String("config"))),
		config.WithZone(c.String("zone")),
	)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx := context.Background()
	factory := newPersistenceFactory(cfg, logger)
	defer factory.Close()

	clusterMetadataManager, err := factory.NewClusterMetadataManager()
	if err != nil {
		return fmt.Errorf("creating cluster metadata manager: %w", err)
	}
	defer clusterMetadataManager.Close()

	currentCluster := cfg.ClusterMetadata.CurrentClusterName
	record, err := clusterMetadataManager.GetClusterMetadata(ctx, &persistence.GetClusterMetadataRequest{
		ClusterName: currentCluster,
	})
	if err != nil {
		return fmt.Errorf("reading cluster metadata for %v: %w", currentCluster, err)
	}

	oldCount := record.HistoryShardCount
	newCount := cfg.Persistence.NumHistoryShards
	dryRun := c.Bool("dry-run")
	logger.Info("Resharding history tasks.",
		tag.NewInt32("from-shard-count", oldCount),
		tag.NewInt32("to-shard-count", newCount),
		tag.NewBoolTag("dry-run", dryRun))

	executionManager, err := factory.NewExecutionManager()
	if err != nil {
		return fmt.Errorf("creating execution manager: %w", err)
	}
	defer executionManager.Close()

	shardManager, err := factory.NewShardManager()
	if err != nil {
		return fmt.Errorf("creating shard manager: %w", err)
	}
	defer shardManager.Close()

	taskStore, err := newTaskStore(cfg, logger)
	if err != nil {
		return err
	}
	defer func() { _ = taskStore.Close() }()

	resharder := &reshard.Resharder{
		ExecutionManager: executionManager,
		ShardManager:     shardManager,
		TaskStore:        taskStore,
		Registry:         reshard.PersistedCategories(),
		OldShardCount:    oldCount,
		NewShardCount:    newCount,
		BatchSize:        c.Int("batch-size"),
		Concurrency:      c.Int("concurrency"),
		DryRun:           dryRun,
		Logger:           logger,
	}

	stats, err := resharder.Run(ctx)
	// Report progress even on failure: a partial run is resumable, and the operator needs to know
	// whether anything moved before deciding what to do next.
	logger.Info("Resharding finished.",
		tag.NewInt("shards-prepared", stats.ShardsPrepared),
		tag.NewInt("tasks-scanned", stats.TasksScanned),
		tag.NewInt("tasks-moved", stats.TasksMoved))
	if err != nil {
		return fmt.Errorf("resharding: %w", err)
	}

	if dryRun {
		logger.Info("Dry run: nothing was changed. Re-run without --dry-run to apply it.")
		return nil
	}
	logger.Info("Start the cluster now. It will persist the new shard count on startup, which " +
		"requires persistence.allowHistoryShardCountIncrease to be set.")
	return nil
}

// newTaskStore opens a second, direct handle to the default datastore.
//
// The factory above hands out an ExecutionManager, which is store-agnostic and so cannot express the
// single UPDATE a task move has to be. Reads still go through it; only the move needs this.
func newTaskStore(cfg *config.Config, logger log.Logger) (sqlplugin.DB, error) {
	name := cfg.Persistence.DefaultStore
	store, ok := cfg.Persistence.DataStores[name]
	if !ok {
		return nil, fmt.Errorf("no datastore named %v in the config", name)
	}
	if store.SQL == nil {
		return nil, fmt.Errorf("datastore %v is not a SQL store, which is the only kind this tool "+
			"can reshard: moving a task must be one statement", name)
	}

	db, err := sql.NewSQLDB(sqlplugin.DbKindMain, store.SQL, resolver.NewNoopResolver(), logger, metrics.NoopMetricsHandler)
	if err != nil {
		return nil, fmt.Errorf("connecting to datastore %v: %w", name, err)
	}
	return db, nil
}

func newPersistenceFactory(cfg *config.Config, logger log.Logger) persistenceClient.Factory {
	clusterName := persistenceClient.ClusterName(cfg.ClusterMetadata.CurrentClusterName)
	serializer := serialization.NewSerializer()
	metricsHandler := metrics.NoopMetricsHandler.WithTags(metrics.ServiceNameTag(primitives.ServerService))

	dataStoreFactory := persistenceClient.DataStoreFactoryProvider(
		clusterName,
		resolver.NewNoopResolver(),
		&cfg.Persistence,
		nil,
		logger,
		metricsHandler,
		telemetry.NoopTracerProvider,
		serializer,
	)
	return persistenceClient.FactoryProvider(persistenceClient.NewFactoryParams{
		DataStoreFactory: dataStoreFactory,
		Cfg:              &cfg.Persistence,
		ClusterName:      clusterName,
		MetricsHandler:   metricsHandler,
		Logger:           logger,
		Serializer:       serializer,
	})
}
