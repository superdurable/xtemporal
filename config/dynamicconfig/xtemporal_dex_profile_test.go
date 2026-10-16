package dynamicconfig_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	_ "github.com/superdurable/xtemporal/chasm/lib/activity"
	_ "github.com/superdurable/xtemporal/chasm/lib/nexusoperation"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
)

func TestDexProfileKeysAndValues(t *testing.T) {
	contents, err := os.ReadFile("xtemporal-dex-profile.yaml")
	require.NoError(t, err)
	loaded := dynamicconfig.LoadYamlFile(contents)
	require.NoError(t, loaded.Err())
	require.Empty(t, loaded.Warnings)

	expected := map[string]any{
		"xtemporal.frontend.disableCountWorkflowExecutions": true,
		"frontend.disableListVisibilityByFilter":            true,
		"frontend.visibilityMaxPageSize":                    100,
		"system.visibilityDisableOrderByClause":             true,
		"system.enableDeployments":                          false,
		"system.enableDeploymentVersions":                   false,
		"frontend.workerVersioningDataAPIs":                 false,
		"frontend.workerVersioningWorkflowAPIs":             false,
		"frontend.workerVersioningRuleAPIs":                 false,
		"frontend.MaxConcurrentBatchOperationPerNamespace":  0,
		"worker.enableNamespaceBatcher":                     false,
		"activity.enableStandalone":                         false,
		"nexusoperation.enableStandalone":                   false,
	}
	require.Len(t, loaded.Map, len(expected))
	for key, value := range expected {
		values, ok := loaded.Map[dynamicconfig.MakeKey(key)]
		require.True(t, ok, "missing key %s", key)
		require.Len(t, values, 1, "key %s", key)
		require.Equal(t, value, values[0].Value, "key %s", key)
	}
}
