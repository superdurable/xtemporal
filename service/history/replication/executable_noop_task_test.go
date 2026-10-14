package replication

import (
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/superdurable/xtemporal/client"
	"github.com/superdurable/xtemporal/common/cluster"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/service/history/shard"
	"go.temporal.io/api/serviceerror"
	"go.uber.org/mock/gomock"
)

type (
	executableNoopTaskSuite struct {
		suite.Suite
		*require.Assertions

		controller              *gomock.Controller
		clusterMetadata         *cluster.MockMetadata
		clientBean              *client.MockBean
		shardController         *shard.MockController
		namespaceCache          *namespace.MockRegistry
		metricsHandler          metrics.Handler
		logger                  log.Logger
		eagerNamespaceRefresher *MockEagerNamespaceRefresher

		task *ExecutableNoopTask
	}
)

func TestExecutableNoopTaskSuite(t *testing.T) {
	s := new(executableNoopTaskSuite)
	suite.Run(t, s)
}

func (s *executableNoopTaskSuite) SetupSuite() {
	s.Assertions = require.New(s.T())
}

func (s *executableNoopTaskSuite) TearDownSuite() {

}

func (s *executableNoopTaskSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.clusterMetadata = cluster.NewMockMetadata(s.controller)
	s.clientBean = client.NewMockBean(s.controller)
	s.shardController = shard.NewMockController(s.controller)
	s.namespaceCache = namespace.NewMockRegistry(s.controller)
	s.metricsHandler = metrics.NoopMetricsHandler
	s.logger = log.NewNoopLogger()
	s.eagerNamespaceRefresher = NewMockEagerNamespaceRefresher(s.controller)

	s.task = NewExecutableNoopTask(
		ProcessToolBox{
			ClusterMetadata:         s.clusterMetadata,
			ClientBean:              s.clientBean,
			ShardController:         s.shardController,
			NamespaceCache:          s.namespaceCache,
			MetricsHandler:          s.metricsHandler,
			Logger:                  s.logger,
			EagerNamespaceRefresher: s.eagerNamespaceRefresher,
			DLQWriter:               NoopDLQWriter{},
		},
		rand.Int63(),
		time.Unix(0, rand.Int63()),
		"sourceCluster",
		ClusterShardKey{
			ClusterID: int32(cluster.TestCurrentClusterInitialFailoverVersion),
			ShardID:   rand.Int31(),
		},
	)
}

func (s *executableNoopTaskSuite) TearDownTest() {
	s.controller.Finish()
}

func (s *executableNoopTaskSuite) TestExecute() {
	err := s.task.Execute()
	s.NoError(err)
}

func (s *executableNoopTaskSuite) TestHandleErr() {
	err := errors.New("OwO")
	s.Equal(err, s.task.HandleErr(err))

	err = serviceerror.NewUnavailable("")
	s.Equal(err, s.task.HandleErr(err))
}

func (s *executableNoopTaskSuite) TestMarkPoisonPill() {
	err := s.task.MarkPoisonPill()
	s.NoError(err)
}
