package tests

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin/mysql"
	mysqlversionV8 "github.com/superdurable/xtemporal/schema/mysql/v8"
	"github.com/superdurable/xtemporal/temporal/environment"
	"github.com/superdurable/xtemporal/tools/sql/clitest"
)

func TestMySQLConnTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, clitest.NewSQLConnTestSuite(
		environment.GetMySQLAddress(),
		strconv.Itoa(environment.GetMySQLPort()),
		mysql.PluginName,
		testMySQLQuery,
	))
}

func TestMySQLHandlerTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, clitest.NewHandlerTestSuite(
		environment.GetMySQLAddress(),
		strconv.Itoa(environment.GetMySQLPort()),
		mysql.PluginName,
	))
}

func TestMySQLSetupSchemaTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, clitest.NewSetupSchemaTestSuite(
		environment.GetMySQLAddress(),
		strconv.Itoa(environment.GetMySQLPort()),
		mysql.PluginName,
		testMySQLQuery,
	))
}

func TestMySQLUpdateSchemaTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, clitest.NewUpdateSchemaTestSuite(
		environment.GetMySQLAddress(),
		strconv.Itoa(environment.GetMySQLPort()),
		mysql.PluginName,
		testMySQLQuery,
		testMySQLExecutionSchemaVersionDir,
		mysqlversionV8.Version,
		testMySQLVisibilitySchemaVersionDir,
		mysqlversionV8.VisibilityVersion,
	))
}

func TestMySQLVersionTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, clitest.NewVersionTestSuite(
		environment.GetMySQLAddress(),
		strconv.Itoa(environment.GetMySQLPort()),
		mysql.PluginName,
		testMySQLExecutionSchemaFile,
		testMySQLVisibilitySchemaFile,
	))
}
