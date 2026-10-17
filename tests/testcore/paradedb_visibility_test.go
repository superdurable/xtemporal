package testcore

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/config"
)

// xtemporal: Reject a bad Dex endpoint before creating either test database.
func TestSetupParadeDBVisibilityInvalidPort(t *testing.T) {
	t.Setenv("PARADEDB_PORT", "invalid")
	cleanup, err := setupParadeDBVisibility(&config.Persistence{})
	require.ErrorContains(t, err, "invalid PARADEDB_PORT")
	require.Nil(t, cleanup)
}
