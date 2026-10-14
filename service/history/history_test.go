package history

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"github.com/superdurable/xtemporal/api/adminservice/v1"
	enumsspb "github.com/superdurable/xtemporal/api/enums/v1"
	historyspb "github.com/superdurable/xtemporal/api/history/v1"
	"github.com/superdurable/xtemporal/api/historyservice/v1"
	persistencespb "github.com/superdurable/xtemporal/api/persistence/v1"
	"github.com/superdurable/xtemporal/chasm"
	chasmworkflow "github.com/superdurable/xtemporal/chasm/lib/workflow"
	"github.com/superdurable/xtemporal/common"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/visibility/manager"
	"github.com/superdurable/xtemporal/service/history/api/forcedeleteworkflowexecution"
	"github.com/superdurable/xtemporal/service/history/tests"
	"go.uber.org/mock/gomock"
)

type (
	historyAPISuite struct {
		suite.Suite
		*require.Assertions

		controller         *gomock.Controller
		mockExecutionMgr   *persistence.MockExecutionManager
		mockVisibilityMgr  *manager.MockVisibilityManager
		mockNamespaceCache *namespace.MockRegistry

		chasmRegistry *chasm.Registry
		logger        log.Logger
	}
)

func TestHistoryAPISuite(t *testing.T) {
	s := new(historyAPISuite)
	suite.Run(t, s)
}

func (s *historyAPISuite) SetupSuite() {

}

func (s *historyAPISuite) TearDownSuite() {
}

func (s *historyAPISuite) SetupTest() {
	s.Assertions = require.New(s.T())

	s.controller = gomock.NewController(s.T())
	s.mockExecutionMgr = persistence.NewMockExecutionManager(s.controller)
	s.mockVisibilityMgr = manager.NewMockVisibilityManager(s.controller)
	s.mockNamespaceCache = namespace.NewMockRegistry(s.controller)
	s.mockNamespaceCache.EXPECT().GetNamespaceByID(tests.NamespaceID).Return(tests.LocalNamespaceEntry, nil).AnyTimes()
	s.mockNamespaceCache.EXPECT().GetNamespace(tests.Namespace).Return(tests.LocalNamespaceEntry, nil).AnyTimes()

	s.logger = log.NewTestLogger()

	s.chasmRegistry = chasm.NewRegistry(s.logger)
	err := s.chasmRegistry.Register(chasmworkflow.NewLibrary(chasmworkflow.NewRegistry()))
	s.NoError(err)
}

func (s *historyAPISuite) TearDownTest() {
	s.controller.Finish()
}
func (s *historyAPISuite) TestDeleteWorkflowExecution_DeleteCurrentExecution() {
	execution := commonpb.WorkflowExecution{
		WorkflowId: "workflowID",
	}

	shardID := common.WorkflowIDToHistoryShard(
		tests.NamespaceID.String(),
		execution.GetWorkflowId(),
		1,
	)

	request := &historyservice.ForceDeleteWorkflowExecutionRequest{
		NamespaceId: tests.NamespaceID.String(),
		ArchetypeId: chasm.WorkflowArchetypeID,
		Request: &adminservice.DeleteWorkflowExecutionRequest{
			Execution: &execution,
			Archetype: chasm.WorkflowArchetype,
		},
	}

	s.mockNamespaceCache.EXPECT().GetNamespaceID(tests.Namespace).Return(tests.NamespaceID, nil).AnyTimes()
	s.mockVisibilityMgr.EXPECT().DeleteWorkflowExecution(gomock.Any(), gomock.Any()).AnyTimes()

	s.mockExecutionMgr.EXPECT().GetCurrentExecution(gomock.Any(), gomock.Any()).Return(nil, errors.New("some random error"))
	resp, err := forcedeleteworkflowexecution.Invoke(
		context.Background(),
		request,
		shardID,
		s.chasmRegistry,
		s.mockExecutionMgr,
		s.mockVisibilityMgr,
		s.logger,
	)
	s.Nil(resp)
	s.Error(err)

	mutableState := &persistencespb.WorkflowMutableState{
		ExecutionInfo: &persistencespb.WorkflowExecutionInfo{
			VersionHistories: &historyspb.VersionHistories{
				CurrentVersionHistoryIndex: 0,
				Histories: []*historyspb.VersionHistory{
					{BranchToken: []byte("branch1")},
					{BranchToken: []byte("branch2")},
					{BranchToken: []byte("branch3")},
					{BranchToken: []byte{}},
				},
			},
		},
	}

	runID := uuid.NewString()
	s.mockExecutionMgr.EXPECT().GetCurrentExecution(gomock.Any(), gomock.Any()).Return(&persistence.GetCurrentExecutionResponse{
		StartRequestID: uuid.NewString(),
		RunID:          runID,
		State:          enumsspb.WORKFLOW_EXECUTION_STATE_COMPLETED,
		Status:         enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED,
	}, nil)
	s.mockExecutionMgr.EXPECT().GetWorkflowExecution(gomock.Any(), &persistence.GetWorkflowExecutionRequest{
		ShardID:     shardID,
		NamespaceID: tests.NamespaceID.String(),
		WorkflowID:  execution.WorkflowId,
		RunID:       runID,
		ArchetypeID: chasm.WorkflowArchetypeID,
	}).Return(&persistence.GetWorkflowExecutionResponse{State: mutableState}, nil)
	s.mockExecutionMgr.EXPECT().DeleteCurrentWorkflowExecution(gomock.Any(), &persistence.DeleteCurrentWorkflowExecutionRequest{
		ShardID:     shardID,
		NamespaceID: tests.NamespaceID.String(),
		WorkflowID:  execution.WorkflowId,
		RunID:       runID,
		ArchetypeID: chasm.WorkflowArchetypeID,
	}).Return(nil)
	s.mockExecutionMgr.EXPECT().DeleteWorkflowExecution(gomock.Any(), &persistence.DeleteWorkflowExecutionRequest{
		ShardID:     shardID,
		NamespaceID: tests.NamespaceID.String(),
		WorkflowID:  execution.WorkflowId,
		RunID:       runID,
		ArchetypeID: chasm.WorkflowArchetypeID,
	}).Return(nil)
	s.mockExecutionMgr.EXPECT().DeleteHistoryBranch(gomock.Any(), gomock.Any()).Times(3)

	_, err = forcedeleteworkflowexecution.Invoke(
		context.Background(),
		request,
		shardID,
		s.chasmRegistry,
		s.mockExecutionMgr,
		s.mockVisibilityMgr,
		s.logger,
	)
	s.NoError(err)
}

func (s *historyAPISuite) TestDeleteWorkflowExecution_LoadMutableStateFailed() {
	execution := commonpb.WorkflowExecution{
		WorkflowId: "workflowID",
		RunId:      uuid.NewString(),
	}

	shardID := common.WorkflowIDToHistoryShard(
		tests.NamespaceID.String(),
		execution.GetWorkflowId(),
		1,
	)

	request := &historyservice.ForceDeleteWorkflowExecutionRequest{
		NamespaceId: tests.NamespaceID.String(),
		ArchetypeId: chasm.WorkflowArchetypeID,
		Request: &adminservice.DeleteWorkflowExecutionRequest{
			Execution: &execution,
			Archetype: chasm.WorkflowArchetype,
		},
	}

	s.mockNamespaceCache.EXPECT().GetNamespaceID(tests.Namespace).Return(tests.NamespaceID, nil).AnyTimes()
	s.mockVisibilityMgr.EXPECT().DeleteWorkflowExecution(gomock.Any(), gomock.Any()).AnyTimes()

	s.mockExecutionMgr.EXPECT().GetWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil, errors.New("some random error"))
	s.mockExecutionMgr.EXPECT().DeleteCurrentWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil)
	s.mockExecutionMgr.EXPECT().DeleteWorkflowExecution(gomock.Any(), gomock.Any()).Return(nil)

	_, err := forcedeleteworkflowexecution.Invoke(
		context.Background(),
		request,
		shardID,
		s.chasmRegistry,
		s.mockExecutionMgr,
		s.mockVisibilityMgr,
		s.logger,
	)
	s.NoError(err)
}
