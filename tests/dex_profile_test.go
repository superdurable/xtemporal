package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/tests/testcore"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
)

func TestDexProfileCountWorkflowExecutionsDisabled(t *testing.T) {
	env := testcore.NewEnv(t, testcore.WithDynamicConfig(dynamicconfig.DisableCountWorkflowExecutions, true))
	resp, err := env.FrontendClient().CountWorkflowExecutions(t.Context(), &workflowservice.CountWorkflowExecutionsRequest{
		Namespace: env.Namespace().String(),
	})
	require.Nil(t, resp)
	var unimplemented *serviceerror.Unimplemented
	require.ErrorAs(t, err, &unimplemented)
	require.EqualError(t, err, "CountWorkflowExecutions is disabled for this namespace")
}
