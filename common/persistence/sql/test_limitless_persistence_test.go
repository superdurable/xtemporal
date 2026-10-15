package sql

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/log"
)

func TestLimitlessVisibilityConfig(t *testing.T) {
	t.Setenv("LIMITLESS_VISIBILITY_HOST", "127.0.0.1")
	t.Setenv("LIMITLESS_VISIBILITY_PORT", "5432")
	t.Setenv("LIMITLESS_VISIBILITY_USER", "temporal")
	t.Setenv("LIMITLESS_VISIBILITY_PASSWORD", "password")
	t.Setenv("LIMITLESS_VISIBILITY_SSLMODE", "")

	cfg, err := limitlessVisibilityConfig()
	require.NoError(t, err)
	require.Equal(t, "postgres12_pgx", cfg.PluginName)
	require.Equal(t, "127.0.0.1:5432", cfg.ConnectAddr)
	require.Equal(t, "disable", cfg.ConnectAttributes["sslmode"])
	require.NotEmpty(t, cfg.DatabaseName)
	t.Setenv("LIMITLESS_TEST", "aurora")
	cluster := NewTestCluster("postgres12_limitless", "limitless", "temporal", "password", "127.0.0.1", 5432, nil, "", nil, log.NewNoopLogger())
	persistenceCfg := cluster.Config()
	require.Equal(t, "limitless", persistenceCfg.DataStores[persistenceCfg.DefaultStore].SQL.DatabaseName)
	require.Equal(t, cfg.PluginName, persistenceCfg.DataStores[persistenceCfg.VisibilityStore].SQL.PluginName)
	require.NotEqual(t, "limitless", persistenceCfg.DataStores[persistenceCfg.VisibilityStore].SQL.DatabaseName)

	t.Setenv("LIMITLESS_VISIBILITY_HOST", "")
	_, err = limitlessVisibilityConfig()
	require.ErrorContains(t, err, "LIMITLESS_VISIBILITY_HOST is required")
	t.Setenv("LIMITLESS_VISIBILITY_HOST", "127.0.0.1")
	t.Setenv("LIMITLESS_VISIBILITY_PORT", "invalid")
	_, err = limitlessVisibilityConfig()
	require.ErrorContains(t, err, "LIMITLESS_VISIBILITY_PORT must be a valid port")
}
