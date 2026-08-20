package tests

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin/postgresql"
	postgresqlversionV12 "github.com/superdurable/temporal-server/schema/postgresql/v12"
	"github.com/superdurable/temporal-server/temporal/environment"
	"github.com/superdurable/temporal-server/tools/sql/clitest"
)

type PostgresqlSuite struct {
	suite.Suite
	pluginName string
}

func (p *PostgresqlSuite) TestPostgreSQLConnTestSuite() {
	suite.Run(p.T(), clitest.NewSQLConnTestSuite(
		environment.GetPostgreSQLAddress(),
		strconv.Itoa(environment.GetPostgreSQLPort()),
		p.pluginName,
		testPostgreSQLQuery,
	))
}

func (p *PostgresqlSuite) TestPostgreSQLHandlerTestSuite() {
	suite.Run(p.T(), clitest.NewHandlerTestSuite(
		environment.GetPostgreSQLAddress(),
		strconv.Itoa(environment.GetPostgreSQLPort()),
		p.pluginName,
	))
}

func (p *PostgresqlSuite) TestPostgreSQLSetupSchemaTestSuite() {
	suite.Run(p.T(), clitest.NewSetupSchemaTestSuite(
		environment.GetPostgreSQLAddress(),
		strconv.Itoa(environment.GetPostgreSQLPort()),
		p.pluginName,
		testPostgreSQLQuery,
	))
}

func (p *PostgresqlSuite) TestPostgreSQLUpdateSchemaTestSuite() {
	suite.Run(p.T(), clitest.NewUpdateSchemaTestSuite(
		environment.GetPostgreSQLAddress(),
		strconv.Itoa(environment.GetPostgreSQLPort()),
		p.pluginName,
		testPostgreSQLQuery,
		testPostgreSQLExecutionSchemaVersionDir,
		postgresqlversionV12.Version,
		testPostgreSQLVisibilitySchemaVersionDir,
		postgresqlversionV12.VisibilityVersion,
	))
}

func (p *PostgresqlSuite) TestPostgreSQLVersionTestSuite() {
	suite.Run(p.T(), clitest.NewVersionTestSuite(
		environment.GetPostgreSQLAddress(),
		strconv.Itoa(environment.GetPostgreSQLPort()),
		p.pluginName,
		testPostgreSQLExecutionSchemaFile,
		testPostgreSQLVisibilitySchemaFile,
	))
}

func TestPostgres(t *testing.T) {
	t.Parallel()
	s := &PostgresqlSuite{pluginName: postgresql.PluginName}
	suite.Run(t, s)
}

func TestPostgresPGX(t *testing.T) {
	t.Parallel()
	s := &PostgresqlSuite{pluginName: postgresql.PluginNamePGX}
	suite.Run(t, s)
}
