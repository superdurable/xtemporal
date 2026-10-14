package tests

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
	persistencetests "github.com/superdurable/xtemporal/common/persistence/persistence-tests"
)

func TestParadeDBVisibilityPersistence(t *testing.T) {
	if os.Getenv("PARADEDB_TEST") != "1" {
		t.Skip("set PARADEDB_TEST=1 to run against ParadeDB")
	}
	suite.Run(t, &VisibilityPersistenceSuite{
		TestBase: persistencetests.NewTestBaseWithSQL(
			persistencetests.GetPostgreSQLTestClusterOption("paradedb", nil),
		),
	})
}
