package visibility

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
)

// xtemporal: The Dex store accepts list values through the configured gate.
func TestParadeDBDexAllowListForValidation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		allowList := dynamicconfig.GetBoolPropertyFnFilteredByNamespace(enabled)
		got := AllowListForValidation([]string{ParadeDBDexStoreName}, allowList)
		require.Equal(t, enabled, got("test-namespace"))
	}
	require.True(t, UsesClusterSearchAttributes(ParadeDBDexStoreName))
	require.True(t, UsesClusterSearchAttributes("elasticsearch"))
	require.False(t, UsesClusterSearchAttributes("postgres12_pgx"))
}
