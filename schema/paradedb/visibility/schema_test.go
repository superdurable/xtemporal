package visibility

import (
	"bytes"
	"io/fs"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	dbschema "github.com/superdurable/xtemporal/schema"
)

func TestFullAndVersionedSchemaMatch(t *testing.T) {
	assets := dbschema.Assets()
	full, err := fs.ReadFile(assets, "paradedb/visibility/schema.sql")
	require.NoError(t, err)
	versioned, err := fs.ReadFile(assets, "paradedb/visibility/versioned/v2.0/schema.sql")
	require.NoError(t, err)
	require.True(t, bytes.Equal(full, versioned))
	benchmark, err := os.ReadFile("../../../develop/docker-compose/paradedb/bench/dex-profile-schema.sql")
	require.NoError(t, err)
	require.True(t, bytes.Equal(full, benchmark))
}
