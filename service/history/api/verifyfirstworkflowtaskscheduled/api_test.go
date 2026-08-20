package verifyfirstworkflowtaskscheduled

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	enumsspb "github.com/superdurable/temporal-server/api/enums/v1"
	"github.com/superdurable/temporal-server/api/historyservice/v1"
	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	workflowspb "github.com/superdurable/temporal-server/api/workflow/v1"
	"github.com/superdurable/temporal-server/common"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/common/payloads"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/service/history/api"
	"github.com/superdurable/temporal-server/service/history/events"
	"github.com/superdurable/temporal-server/service/history/hsm"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/shard"
	"github.com/superdurable/temporal-server/service/history/tests"
	"github.com/superdurable/temporal-server/service/history/workflow"
	wcache "github.com/superdurable/temporal-server/service/history/workflow/cache"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/durationpb"
)

var defaultWorkflowTaskCompletionLimits = historyi.WorkflowTaskCompletionLimits{MaxResetPoints: primitives.DefaultHistoryMaxAutoResetPoints, MaxSearchAttributeValueSize: 2048}

type (
	VerifyFirstWorkflowTaskScheduledSuite struct {
		*require.Assertions
		suite.Suite

		controller                 *gomock.Controller
		mockEventsCache            *events.MockCache
		mockExecutionMgr           *persistence.MockExecutionManager
		shardContext               *shard.ContextTest
		workflowConsistencyChecker api.WorkflowConsistencyChecker

		logger log.Logger
	}
)

func TestVerifyFirstWorkflowTaskScheduledSuite(t *testing.T) {
	suite.Run(t, new(VerifyFirstWorkflowTaskScheduledSuite))
}

func (s *VerifyFirstWorkflowTaskScheduledSuite) SetupTest() {
	s.Assertions = require.New(s.T())
	s.controller = gomock.NewController(s.T())

	config := tests.NewDynamicConfig()
	s.shardContext = shard.NewTestContext(
		s.controller,
		&persistencespb.ShardInfo{
			ShardId: 1,
			RangeId: 1,
		},
		config,
	)

	reg := hsm.NewRegistry()
	err := workflow.RegisterStateMachine(reg)
	s.NoError(err)
	s.shardContext.SetStateMachineRegistry(reg)

	mockNamespaceCache := s.shardContext.Resource.NamespaceCache
	mockNamespaceCache.EXPECT().GetNamespaceByID(tests.NamespaceID).Return(tests.LocalNamespaceEntry, nil).AnyTimes()
	s.mockExecutionMgr = s.shardContext.Resource.ExecutionMgr
	mockClusterMetadata := s.shardContext.Resource.ClusterMetadata
	mockClusterMetadata.EXPECT().GetClusterID().Return(int64(1)).AnyTimes()
	mockClusterMetadata.EXPECT().GetCurrentClusterName().Return(cluster.TestCurrentClusterName).AnyTimes()
	mockClusterMetadata.EXPECT().ClusterNameForFailoverVersion(false, common.EmptyVersion).Return(cluster.TestCurrentClusterName).AnyTimes()
	mockClusterMetadata.EXPECT().ClusterNameForFailoverVersion(true, tests.Version).Return(cluster.TestCurrentClusterName).AnyTimes()

	s.workflowConsistencyChecker = api.NewWorkflowConsistencyChecker(
		s.shardContext,
		wcache.NewHostLevelCache(s.shardContext.GetConfig(), s.shardContext.GetLogger(), metrics.NoopMetricsHandler))
	s.mockEventsCache = s.shardContext.MockEventsCache
	s.mockEventsCache.EXPECT().PutEvent(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger = s.shardContext.GetLogger()
}

func (s *VerifyFirstWorkflowTaskScheduledSuite) TearDownTest() {
	s.controller.Finish()
}

func (s *VerifyFirstWorkflowTaskScheduledSuite) TestVerifyFirstWorkflowTaskScheduled_WorkflowNotFound() {
	request := &historyservice.VerifyFirstWorkflowTaskScheduledRequest{
		NamespaceId: tests.NamespaceID.String(),
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: tests.WorkflowID,
			RunId:      tests.RunID,
		},
	}

	s.mockExecutionMgr.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil, &serviceerror.NotFound{})

	err := Invoke(context.Background(), request, s.workflowConsistencyChecker)
	s.IsType(&serviceerror.NotFound{}, err)
}

func (s *VerifyFirstWorkflowTaskScheduledSuite) TestVerifyFirstWorkflowTaskScheduled_WorkflowCompleted() {
	request := &historyservice.VerifyFirstWorkflowTaskScheduledRequest{
		NamespaceId: tests.NamespaceID.String(),
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: tests.WorkflowID,
			RunId:      tests.RunID,
		},
	}

	ms := workflow.TestGlobalMutableState(s.shardContext, s.mockEventsCache, s.logger, tests.Version, tests.WorkflowID, tests.RunID)

	addWorkflowExecutionStartedEventWithParent(ms,
		&commonpb.WorkflowExecution{
			WorkflowId: tests.WorkflowID,
			RunId:      tests.RunID,
		}, "wType", "testTaskQueue", payloads.EncodeString("input"),
		25*time.Second, 20*time.Second, 200*time.Second, nil, "identity")

	_, err := ms.AddTimeoutWorkflowEvent(
		ms.GetNextEventID(),
		enumspb.RETRY_STATE_RETRY_POLICY_NOT_SET,
		uuid.NewString(),
	)
	s.NoError(err)

	wfMs := workflow.TestCloneToProto(context.Background(), ms)
	gwmsResponse := &persistence.GetWorkflowExecutionResponse{State: wfMs}
	s.mockExecutionMgr.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(gwmsResponse, nil)

	err = Invoke(context.Background(), request, s.workflowConsistencyChecker)
	s.NoError(err)
}

func (s *VerifyFirstWorkflowTaskScheduledSuite) TestVerifyFirstWorkflowTaskScheduled_WorkflowZombie() {
	request := &historyservice.VerifyFirstWorkflowTaskScheduledRequest{
		NamespaceId: tests.NamespaceID.String(),
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: tests.WorkflowID,
			RunId:      tests.RunID,
		},
	}

	ms := workflow.TestGlobalMutableState(s.shardContext, s.mockEventsCache, s.logger, tests.Version, tests.WorkflowID, tests.RunID)

	addWorkflowExecutionStartedEventWithParent(ms,
		&commonpb.WorkflowExecution{
			WorkflowId: tests.WorkflowID,
			RunId:      tests.RunID,
		}, "wType", "testTaskQueue", payloads.EncodeString("input"),
		25*time.Second, 20*time.Second, 200*time.Second, nil, "identity")

	// zombie state should be treated as open
	_, err := ms.UpdateWorkflowStateStatus(
		enumsspb.WORKFLOW_EXECUTION_STATE_ZOMBIE,
		enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
	)
	s.NoError(err)

	wfMs := workflow.TestCloneToProto(context.Background(), ms)
	gwmsResponse := &persistence.GetWorkflowExecutionResponse{State: wfMs}
	s.mockExecutionMgr.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(gwmsResponse, nil)

	err = Invoke(context.Background(), request, s.workflowConsistencyChecker)
	s.IsType(&serviceerror.WorkflowNotReady{}, err)
}

func (s *VerifyFirstWorkflowTaskScheduledSuite) TestVerifyFirstWorkflowTaskScheduled_WorkflowRunning_TaskPending() {
	request := &historyservice.VerifyFirstWorkflowTaskScheduledRequest{
		NamespaceId: tests.NamespaceID.String(),
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: tests.WorkflowID,
			RunId:      tests.RunID,
		},
	}

	ms := workflow.TestGlobalMutableState(s.shardContext, s.mockEventsCache, s.logger, tests.Version, tests.WorkflowID, tests.RunID)

	addWorkflowExecutionStartedEventWithParent(ms,
		&commonpb.WorkflowExecution{
			WorkflowId: tests.WorkflowID,
			RunId:      tests.RunID,
		}, "wType", "testTaskQueue", payloads.EncodeString("input"),
		25*time.Second, 20*time.Second, 200*time.Second, nil, "identity")
	_, _ = ms.AddWorkflowTaskScheduledEvent(false, enumsspb.WORKFLOW_TASK_TYPE_NORMAL)

	wfMs := workflow.TestCloneToProto(context.Background(), ms)
	gwmsResponse := &persistence.GetWorkflowExecutionResponse{State: wfMs}
	s.mockExecutionMgr.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(gwmsResponse, nil)

	err := Invoke(context.Background(), request, s.workflowConsistencyChecker)
	s.NoError(err)
}

func (s *VerifyFirstWorkflowTaskScheduledSuite) TestVerifyFirstWorkflowTaskScheduled_WorkflowRunning_TaskProcessed() {
	request := &historyservice.VerifyFirstWorkflowTaskScheduledRequest{
		NamespaceId: tests.NamespaceID.String(),
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: tests.WorkflowID,
			RunId:      tests.RunID,
		},
	}

	ms := workflow.TestGlobalMutableState(s.shardContext, s.mockEventsCache, s.logger, tests.Version, tests.WorkflowID, tests.RunID)

	addWorkflowExecutionStartedEventWithParent(ms,
		&commonpb.WorkflowExecution{
			WorkflowId: tests.WorkflowID,
			RunId:      tests.RunID,
		}, "wType", "testTaskQueue", payloads.EncodeString("input"),
		25*time.Second, 20*time.Second, 200*time.Second, nil, "identity")

	// Schedule WFT
	wt, _ := ms.AddWorkflowTaskScheduledEvent(false, enumsspb.WORKFLOW_TASK_TYPE_NORMAL)

	// Start WFT
	workflowTasksStartEvent, _, _ := ms.AddWorkflowTaskStartedEvent(
		wt.ScheduledEventID,
		tests.RunID,
		&taskqueuepb.TaskQueue{Name: "testTaskQueue"},
		uuid.NewString(),
		nil,
		nil,
		nil,
		false,
		nil,
	)
	wt.StartedEventID = workflowTasksStartEvent.GetEventId()

	// Complete WFT
	workflowTask := ms.GetWorkflowTaskByID(wt.ScheduledEventID)
	s.NotNil(workflowTask)
	s.Equal(wt.StartedEventID, workflowTask.StartedEventID)
	_, _ = ms.AddWorkflowTaskCompletedEvent(workflowTask,
		&workflowservice.RespondWorkflowTaskCompletedRequest{Identity: "some random identity"}, defaultWorkflowTaskCompletionLimits)
	ms.FlushBufferedEvents()

	wfMs := workflow.TestCloneToProto(context.Background(), ms)
	gwmsResponse := &persistence.GetWorkflowExecutionResponse{State: wfMs}
	s.mockExecutionMgr.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(gwmsResponse, nil)

	err := Invoke(context.Background(), request, s.workflowConsistencyChecker)
	s.NoError(err)
}

func addWorkflowExecutionStartedEventWithParent(
	ms historyi.MutableState,
	workflowExecution *commonpb.WorkflowExecution,
	workflowType, taskQueue string,
	input *commonpb.Payloads,
	executionTimeout, runTimeout, taskTimeout time.Duration,
	parentInfo *workflowspb.ParentExecutionInfo,
	identity string,
) *historypb.HistoryEvent {
	startRequest := &workflowservice.StartWorkflowExecutionRequest{
		WorkflowId:               workflowExecution.WorkflowId,
		WorkflowType:             &commonpb.WorkflowType{Name: workflowType},
		TaskQueue:                &taskqueuepb.TaskQueue{Name: taskQueue},
		Input:                    input,
		WorkflowExecutionTimeout: durationpb.New(executionTimeout),
		WorkflowRunTimeout:       durationpb.New(runTimeout),
		WorkflowTaskTimeout:      durationpb.New(taskTimeout),
		Identity:                 identity,
	}

	event, _ := ms.AddWorkflowExecutionStartedEvent(
		workflowExecution,
		&historyservice.StartWorkflowExecutionRequest{
			Attempt:             1,
			NamespaceId:         tests.NamespaceID.String(),
			StartRequest:        startRequest,
			ParentExecutionInfo: parentInfo,
		},
	)

	return event
}
