package cassandra

import (
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/schema/cassandra"
	"github.com/superdurable/temporal-server/tools/common/schema/test"
)

type UpdateSchemaTestSuite struct {
	test.UpdateSchemaTestBase
}

func (s *UpdateSchemaTestSuite) SetupSuite() {
	client, err := newTestCQLClient(systemKeyspace)
	if err != nil {
		s.Logger.Fatal("Error creating CQLClient", tag.Error(err))
	}
	// No ConnectParams needed: Cassandra CLI defaults work without explicit flags.
	s.SetupSuiteBase(client, "", test.ConnectParams{})
}

func (s *UpdateSchemaTestSuite) TearDownSuite() {
	s.TearDownSuiteBase()
}

func (s *UpdateSchemaTestSuite) TestUpdateSchema() {
	client, err := newTestCQLClient(s.DBName)
	s.Nil(err)
	defer client.Close()
	s.RunUpdateSchemaTest(buildCLIOptions(), client, "-k", createTestCQLFileContent(), []string{"events", "tasks"})
}

func (s *UpdateSchemaTestSuite) TestDryrun() {
	client, err := newTestCQLClient(s.DBName)
	s.Nil(err)
	defer client.Close()
	dir := "../../schema/cassandra/temporal/versioned"
	s.RunDryrunTest(buildCLIOptions(), client, "-k", dir, cassandra.Version)
}
