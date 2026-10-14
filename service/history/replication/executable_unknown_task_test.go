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
	executableUnknownTaskSuite struct {
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

		taskID int64
		task   *ExecutableUnknownTask
	}
)

func TestExecutableUnknownTaskSuite(t *testing.T) {
	s := new(executableUnknownTaskSuite)
	suite.Run(t, s)
}

func (s *executableUnknownTaskSuite) SetupSuite() {
	s.Assertions = require.New(s.T())
}

func (s *executableUnknownTaskSuite) TearDownSuite() {

}

func (s *executableUnknownTaskSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.clusterMetadata = cluster.NewMockMetadata(s.controller)
	s.clientBean = client.NewMockBean(s.controller)
	s.shardController = shard.NewMockController(s.controller)
	s.namespaceCache = namespace.NewMockRegistry(s.controller)
	s.metricsHandler = metrics.NoopMetricsHandler
	s.logger = log.NewNoopLogger()
	s.eagerNamespaceRefresher = NewMockEagerNamespaceRefresher(s.controller)

	s.taskID = rand.Int63()
	s.task = NewExecutableUnknownTask(
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
		s.taskID,
		time.Unix(0, rand.Int63()),
		nil,
	)
}

func (s *executableUnknownTaskSuite) TearDownTest() {
	s.controller.Finish()
}

func (s *executableUnknownTaskSuite) TestExecute() {
	err := s.task.Execute()
	s.IsType(serviceerror.NewInvalidArgument(""), err)
}

func (s *executableUnknownTaskSuite) TestHandleErr() {
	err := errors.New("OwO")
	s.Equal(err, s.task.HandleErr(err))

	err = serviceerror.NewUnavailable("")
	s.Equal(err, s.task.HandleErr(err))
}

func (s *executableUnknownTaskSuite) TestMarkPoisonPill() {
	err := s.task.MarkPoisonPill()
	s.NoError(err)
}
