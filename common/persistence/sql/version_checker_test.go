package sql

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/config"
)

// xtemporal: reject schema-tool-only SQL configurations before server database access.
func TestParadeDBSQLVisibilityStoreRejectedBeforeConnecting(t *testing.T) {
	for _, secondary := range []bool{false, true} {
		cfg := config.Persistence{DataStores: map[string]config.DataStore{
			"visibility": {SQL: &config.SQL{PluginName: "paradedb"}},
		}}
		if secondary {
			cfg.SecondaryVisibilityStore = "visibility"
		} else {
			cfg.VisibilityStore = "visibility"
		}
		err := VerifyCompatibleVersion(cfg, nil, nil)
		require.ErrorContains(t, err, "customDatastore (paradedb-dex)")
	}
}
