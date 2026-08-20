package cassandra

import (
	"path"
	"runtime"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/persistence/cassandra"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/common/resolver"
	"github.com/superdurable/temporal-server/temporal/environment"
)

type (
	VersionTestSuite struct {
		*require.Assertions // override suite.Suite.Assertions with require.Assertions; this means that s.NotNil(nil) will stop the test, not merely log an error
		suite.Suite
	}
)

func (s *VersionTestSuite) SetupTest() {
	s.Assertions = require.New(s.T()) // Have to define our overridden assertions in the test setup. If we did it earlier, s.T() will return nil
}

func (s *VersionTestSuite) TestVerifyCompatibleVersion() {
	keyspace := "temporal_ver_test_"
	_, filename, _, ok := runtime.Caller(0)
	s.True(ok)
	root := path.Dir(path.Dir(path.Dir(filename)))
	cqlFile := path.Join(root, "schema/cassandra/temporal/schema.cql")

	defer s.createKeyspace(keyspace)()
	s.Nil(RunTool([]string{
		"./tool", "-k", keyspace, "-q", "setup-schema", "-f", cqlFile, "-version", "10.0", "-o",
	}))

	defaultCfg := config.Cassandra{
		Hosts:    environment.GetCassandraAddress(),
		Port:     environment.GetCassandraPort(),
		User:     "",
		Password: "",
		Keyspace: keyspace,
	}
	cfg := config.Persistence{
		DefaultStore: "default",
		DataStores: map[string]config.DataStore{
			"default": {Cassandra: &defaultCfg},
		},
		TransactionSizeLimit: dynamicconfig.GetIntPropertyFn(primitives.DefaultTransactionSizeLimit),
	}
	s.NoError(cassandra.VerifyCompatibleVersion(cfg, resolver.NewNoopResolver(), log.NewNoopLogger()))
}

func (s *VersionTestSuite) createKeyspace(keyspace string) func() {
	cfg := &CQLClientConfig{
		Hosts:       environment.GetCassandraAddress(),
		Port:        environment.GetCassandraPort(),
		Keyspace:    "system",
		Timeout:     defaultTimeout,
		numReplicas: 1,
	}
	client, err := newCQLClient(cfg, log.NewNoopLogger())
	s.NoError(err)

	err = client.createKeyspace(keyspace)
	if err != nil {
		s.Fail("error creating keyspace, err=%v", err)
	}
	return func() {
		s.NoError(client.dropKeyspace(keyspace))
		client.Close()
	}
}
