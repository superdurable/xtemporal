package await

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/debug"
)

func TestConfig_OverrideAttemptTimeout(t *testing.T) {
	t.Setenv(attemptTimeoutEnvVar, "250ms")

	cfg := newConfig()
	require.Equal(t, 250*time.Millisecond*debug.TimeoutMultiplier, cfg.attemptTimeout)
}
