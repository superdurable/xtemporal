package signalworkflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/workflowservice/v1"
	"github.com/superdurable/xtemporal/api/historyservice/v1"
	persistencespb "github.com/superdurable/xtemporal/api/persistence/v1"
	"github.com/superdurable/xtemporal/chasm"
	"github.com/superdurable/xtemporal/common/clock"
	"github.com/superdurable/xtemporal/common/cluster"
	"github.com/superdurable/xtemporal/common/cluster/clustertest"
	"github.com/superdurable/xtemporal/common/locks"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/service/history/api"
	"github.com/superdurable/xtemporal/service/history/consts"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
	"github.com/superdurable/xtemporal/service/history/tests"
	wcache "github.com/superdurable/xtemporal/service/history/workflow/cache"
	"go.uber.org/mock/gomock"
)

type (
	signalWorkflowSuite struct {
		suite.Suite
		*require.Assertions

		controller        *gomock.Controller
		shardContext      *historyi.MockShardContext
		namespaceRegistry *namespace.MockRegistry

		workflowCache              *wcache.MockCache
		workflowConsistencyChecker api.WorkflowConsistencyChecker

		currentContext      *historyi.MockWorkflowContext
		currentMutableState *historyi.MockMutableState
	}
)

func TestSignalWorkflowSuite(t *testing.T) {
	s := new(signalWorkflowSuite)
	suite.Run(t, s)
}

func (s *signalWorkflowSuite) SetupSuite() {
}

func (s *signalWorkflowSuite) TearDownSuite() {
}

func (s *signalWorkflowSuite) SetupTest() {
	s.Assertions = require.New(s.T())

	s.controller = gomock.NewController(s.T())
	s.namespaceRegistry = namespace.NewMockRegistry(s.controller)
	s.namespaceRegistry.EXPECT().GetNamespaceByID(tests.GlobalNamespaceEntry.ID()).Return(tests.GlobalNamespaceEntry, nil).AnyTimes()

	s.shardContext = historyi.NewMockShardContext(s.controller)
	s.shardContext.EXPECT().GetConfig().Return(tests.NewDynamicConfig()).AnyTimes()
	s.shardContext.EXPECT().GetLogger().Return(log.NewTestLogger()).AnyTimes()
	s.shardContext.EXPECT().GetThrottledLogger().Return(log.NewTestLogger()).AnyTimes()
	s.shardContext.EXPECT().GetMetricsHandler().Return(metrics.NoopMetricsHandler).AnyTimes()
	s.shardContext.EXPECT().GetTimeSource().Return(clock.NewRealTimeSource()).AnyTimes()
	s.shardContext.EXPECT().GetNamespaceRegistry().Return(s.namespaceRegistry).AnyTimes()
	s.shardContext.EXPECT().GetClusterMetadata().Return(clustertest.NewMetadataForTest(cluster.NewTestClusterMetadataConfig(true, true))).AnyTimes()

	s.currentMutableState = historyi.NewMockMutableState(s.controller)
	s.currentMutableState.EXPECT().GetNamespaceEntry().Return(tests.GlobalNamespaceEntry).AnyTimes()
	s.currentMutableState.EXPECT().GetExecutionInfo().Return(&persistencespb.WorkflowExecutionInfo{
		WorkflowId: tests.WorkflowID,
	}).AnyTimes()
	s.currentMutableState.EXPECT().GetExecutionState().Return(&persistencespb.WorkflowExecutionState{
		RunId: tests.RunID,
	}).AnyTimes()

	s.currentContext = historyi.NewMockWorkflowContext(s.controller)
	s.currentContext.EXPECT().LoadMutableState(gomock.Any(), s.shardContext).Return(s.currentMutableState, nil).AnyTimes()

	s.workflowCache = wcache.NewMockCache(s.controller)
	s.workflowCache.EXPECT().GetOrCreateChasmExecution(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), chasm.WorkflowArchetypeID, locks.PriorityHigh).
		Return(s.currentContext, wcache.NoopReleaseFn, nil).AnyTimes()

	s.workflowConsistencyChecker = api.NewWorkflowConsistencyChecker(
		s.shardContext,
		s.workflowCache,
	)
}

func (s *signalWorkflowSuite) TearDownTest() {
	s.controller.Finish()
}

func (s *signalWorkflowSuite) TestSignalWorkflow_WorkflowCloseAttempted() {
	s.currentMutableState.EXPECT().IsWorkflowExecutionRunning().Return(true)
	s.currentMutableState.EXPECT().IsWorkflowCloseAttempted().Return(true)
	s.currentMutableState.EXPECT().HasStartedWorkflowTask().Return(true)

	resp, err := Invoke(
		context.Background(),
		&historyservice.SignalWorkflowExecutionRequest{
			NamespaceId: tests.NamespaceID.String(),
			SignalRequest: &workflowservice.SignalWorkflowExecutionRequest{
				Namespace: tests.Namespace.String(),
				WorkflowExecution: &commonpb.WorkflowExecution{
					WorkflowId: tests.WorkflowID,
					RunId:      tests.RunID,
				},
				SignalName: "signal-name",
				Input:      nil,
			},
		},
		s.shardContext,
		s.workflowConsistencyChecker,
	)
	s.Nil(resp)
	s.ErrorIs(consts.ErrWorkflowClosing, err)
}
