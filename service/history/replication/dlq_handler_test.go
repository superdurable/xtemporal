package replication

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	commonpb "go.temporal.io/api/common/v1"
	"github.com/superdurable/xtemporal/api/adminservice/v1"
	"github.com/superdurable/xtemporal/api/adminservicemock/v1"
	enumsspb "github.com/superdurable/xtemporal/api/enums/v1"
	historyspb "github.com/superdurable/xtemporal/api/history/v1"
	persistencespb "github.com/superdurable/xtemporal/api/persistence/v1"
	replicationspb "github.com/superdurable/xtemporal/api/replication/v1"
	"github.com/superdurable/xtemporal/client"
	"github.com/superdurable/xtemporal/common/cluster"
	"github.com/superdurable/xtemporal/common/definition"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/resourcetest"
	"github.com/superdurable/xtemporal/service/history/configs"
	deletemanager "github.com/superdurable/xtemporal/service/history/deletemanager"
	"github.com/superdurable/xtemporal/service/history/shard"
	"github.com/superdurable/xtemporal/service/history/tasks"
	"github.com/superdurable/xtemporal/service/history/tests"
	wcache "github.com/superdurable/xtemporal/service/history/workflow/cache"
	"go.uber.org/mock/gomock"
)

type (
	dlqHandlerSuite struct {
		suite.Suite
		*require.Assertions
		controller *gomock.Controller

		mockResource     *resourcetest.Test
		mockShard        *shard.ContextTest
		config           *configs.Config
		mockClientBean   *client.MockBean
		adminClient      *adminservicemock.MockAdminServiceClient
		clusterMetadata  *cluster.MockMetadata
		executionManager *persistence.MockExecutionManager
		shardManager     *persistence.MockShardManager
		taskExecutor     *MockTaskExecutor
		taskExecutors    map[string]TaskExecutor
		sourceCluster    string

		replicationMessageHandler *dlqHandlerImpl
	}
)

func TestDLQHandlerSuite(t *testing.T) {
	s := new(dlqHandlerSuite)
	suite.Run(t, s)
}

func (s *dlqHandlerSuite) SetupSuite() {

}

func (s *dlqHandlerSuite) TearDownSuite() {

}

func (s *dlqHandlerSuite) SetupTest() {
	s.Assertions = require.New(s.T())
	s.controller = gomock.NewController(s.T())

	s.mockShard = shard.NewTestContext(
		s.controller,
		&persistencespb.ShardInfo{
			ShardId:                0,
			RangeId:                1,
			ReplicationDlqAckLevel: map[string]int64{cluster.TestAlternativeClusterName: persistence.EmptyQueueMessageID},
		},
		tests.NewDynamicConfig(),
	)
	s.mockResource = s.mockShard.Resource
	s.mockClientBean = s.mockResource.ClientBean
	s.adminClient = s.mockResource.RemoteAdminClient
	s.clusterMetadata = s.mockResource.ClusterMetadata
	s.executionManager = s.mockResource.ExecutionMgr
	s.shardManager = s.mockResource.ShardMgr
	s.config = tests.NewDynamicConfig()
	s.clusterMetadata.EXPECT().GetCurrentClusterName().Return(cluster.TestCurrentClusterName).AnyTimes()
	s.clusterMetadata.EXPECT().GetAllClusterInfo().Return(cluster.TestAllClusterInfo).AnyTimes()
	s.taskExecutors = make(map[string]TaskExecutor)
	s.taskExecutor = NewMockTaskExecutor(s.controller)
	s.sourceCluster = cluster.TestAlternativeClusterName
	s.taskExecutors[s.sourceCluster] = s.taskExecutor

	s.replicationMessageHandler = newDLQHandler(
		s.mockShard,
		deletemanager.NewMockDeleteManager(s.controller),
		wcache.NewMockCache(s.controller),
		s.mockClientBean,
		s.taskExecutors,
		func(params TaskExecutorParams) TaskExecutor {
			return NewTaskExecutor(
				params.RemoteCluster,
				params.Shard,
				params.RemoteHistoryFetcher,
				params.DeleteManager,
				params.WorkflowCache,
			)
		},
	)
}

func (s *dlqHandlerSuite) TearDownTest() {
	s.controller.Finish()
	s.mockShard.StopForTest()
}

func (s *dlqHandlerSuite) TestReadMessages_OK() {
	ctx := context.Background()

	namespaceID := uuid.NewString()
	workflowID := uuid.NewString()
	runID := uuid.NewString()
	taskID := int64(12345)
	version := int64(2333)
	firstEventID := int64(144)
	nextEventID := int64(233)

	lastMessageID := int64(1394)
	pageSize := 1
	pageToken := []byte("some random token")
	dbResp := &persistence.GetHistoryTasksResponse{
		Tasks: []tasks.Task{&tasks.HistoryReplicationTask{
			WorkflowKey: definition.NewWorkflowKey(
				namespaceID,
				workflowID,
				runID,
			),
			Version:      version,
			FirstEventID: firstEventID,
			NextEventID:  nextEventID,
			TaskID:       taskID,
		}},
		NextPageToken: pageToken,
	}

	remoteTask := &replicationspb.ReplicationTask{
		TaskType:     enumsspb.REPLICATION_TASK_TYPE_HISTORY_TASK,
		SourceTaskId: taskID,
		Attributes: &replicationspb.ReplicationTask_HistoryTaskAttributes{
			HistoryTaskAttributes: &replicationspb.HistoryTaskAttributes{
				NamespaceId: namespaceID,
				WorkflowId:  workflowID,
				RunId:       runID,
				VersionHistoryItems: []*historyspb.VersionHistoryItem{{
					Version: version,
					EventId: nextEventID - 1,
				}},
				Events: &commonpb.DataBlob{},
			},
		},
	}

	s.executionManager.EXPECT().GetReplicationTasksFromDLQ(gomock.Any(), &persistence.GetReplicationTasksFromDLQRequest{
		GetHistoryTasksRequest: persistence.GetHistoryTasksRequest{
			ShardID:             s.mockShard.GetShardID(),
			TaskCategory:        tasks.CategoryReplication,
			InclusiveMinTaskKey: tasks.NewImmediateKey(persistence.EmptyQueueMessageID + 1),
			ExclusiveMaxTaskKey: tasks.NewImmediateKey(lastMessageID + 1),
			BatchSize:           pageSize,
			NextPageToken:       pageToken,
		},
		SourceClusterName: s.sourceCluster,
	}).Return(dbResp, nil)

	s.mockClientBean.EXPECT().GetRemoteAdminClient(s.sourceCluster).Return(s.adminClient, nil).AnyTimes()
	s.adminClient.EXPECT().GetDLQReplicationMessages(ctx, gomock.Any()).
		Return(&adminservice.GetDLQReplicationMessagesResponse{
			ReplicationTasks: []*replicationspb.ReplicationTask{remoteTask},
		}, nil)
	taskList, tasksInfo, token, err := s.replicationMessageHandler.GetMessages(ctx, s.sourceCluster, lastMessageID, pageSize, pageToken)
	s.NoError(err)
	s.Equal(pageToken, token)
	s.Equal([]*replicationspb.ReplicationTask{remoteTask}, taskList)
	s.Equal(namespaceID, tasksInfo[0].GetNamespaceId())
	s.Equal(workflowID, tasksInfo[0].GetWorkflowId())
	s.Equal(taskID, tasksInfo[0].GetTaskId())
	s.Equal(version, tasksInfo[0].GetVersion())
	s.Equal(firstEventID, tasksInfo[0].GetFirstEventId())
	s.Equal(nextEventID, tasksInfo[0].GetNextEventId())
}

func (s *dlqHandlerSuite) TestPurgeMessages() {
	lastMessageID := int64(1)

	s.executionManager.EXPECT().RangeDeleteReplicationTaskFromDLQ(
		gomock.Any(),
		&persistence.RangeDeleteReplicationTaskFromDLQRequest{
			RangeCompleteHistoryTasksRequest: persistence.RangeCompleteHistoryTasksRequest{
				ShardID:             s.mockShard.GetShardID(),
				TaskCategory:        tasks.CategoryReplication,
				InclusiveMinTaskKey: tasks.NewImmediateKey(persistence.EmptyQueueMessageID + 1),
				ExclusiveMaxTaskKey: tasks.NewImmediateKey(lastMessageID + 1),
			},
			SourceClusterName: s.sourceCluster,
		}).Return(nil)

	s.shardManager.EXPECT().UpdateShard(gomock.Any(), gomock.Any()).Return(nil)
	err := s.replicationMessageHandler.PurgeMessages(context.Background(), s.sourceCluster, lastMessageID)
	s.NoError(err)
}
func (s *dlqHandlerSuite) TestMergeMessages() {
	ctx := context.Background()

	namespaceID := uuid.NewString()
	workflowID := uuid.NewString()
	runID := uuid.NewString()
	taskID := int64(12345)
	version := int64(2333)
	firstEventID := int64(144)
	nextEventID := int64(233)

	lastMessageID := int64(1394)
	pageSize := 1
	pageToken := []byte("some random token")

	dbResp := &persistence.GetHistoryTasksResponse{
		Tasks: []tasks.Task{&tasks.HistoryReplicationTask{
			WorkflowKey: definition.NewWorkflowKey(
				namespaceID,
				workflowID,
				runID,
			),
			Version:      version,
			FirstEventID: firstEventID,
			NextEventID:  nextEventID,
			TaskID:       taskID,
		}},
		NextPageToken: pageToken,
	}

	remoteTask := &replicationspb.ReplicationTask{
		TaskType:     enumsspb.REPLICATION_TASK_TYPE_HISTORY_TASK,
		SourceTaskId: taskID,
		Attributes: &replicationspb.ReplicationTask_HistoryTaskAttributes{
			HistoryTaskAttributes: &replicationspb.HistoryTaskAttributes{
				NamespaceId: namespaceID,
				WorkflowId:  workflowID,
				RunId:       runID,
				VersionHistoryItems: []*historyspb.VersionHistoryItem{{
					Version: version,
					EventId: nextEventID - 1,
				}},
				Events: &commonpb.DataBlob{},
			},
		},
	}

	s.executionManager.EXPECT().GetReplicationTasksFromDLQ(gomock.Any(), &persistence.GetReplicationTasksFromDLQRequest{
		GetHistoryTasksRequest: persistence.GetHistoryTasksRequest{
			ShardID:             s.mockShard.GetShardID(),
			TaskCategory:        tasks.CategoryReplication,
			InclusiveMinTaskKey: tasks.NewImmediateKey(persistence.EmptyQueueMessageID + 1),
			ExclusiveMaxTaskKey: tasks.NewImmediateKey(lastMessageID + 1),
			BatchSize:           pageSize,
			NextPageToken:       pageToken,
		},
		SourceClusterName: s.sourceCluster,
	}).Return(dbResp, nil)

	s.mockClientBean.EXPECT().GetRemoteAdminClient(s.sourceCluster).Return(s.adminClient, nil).AnyTimes()
	s.adminClient.EXPECT().GetDLQReplicationMessages(ctx, gomock.Any()).
		Return(&adminservice.GetDLQReplicationMessagesResponse{
			ReplicationTasks: []*replicationspb.ReplicationTask{remoteTask},
		}, nil)
	s.taskExecutor.EXPECT().Execute(gomock.Any(), remoteTask, true).Return(nil)
	s.executionManager.EXPECT().RangeDeleteReplicationTaskFromDLQ(gomock.Any(), &persistence.RangeDeleteReplicationTaskFromDLQRequest{
		RangeCompleteHistoryTasksRequest: persistence.RangeCompleteHistoryTasksRequest{
			ShardID:             s.mockShard.GetShardID(),
			TaskCategory:        tasks.CategoryReplication,
			InclusiveMinTaskKey: tasks.NewImmediateKey(persistence.EmptyQueueMessageID + 1),
			ExclusiveMaxTaskKey: tasks.NewImmediateKey(lastMessageID + 1),
		},
		SourceClusterName: s.sourceCluster,
	}).Return(nil)

	s.shardManager.EXPECT().UpdateShard(gomock.Any(), gomock.Any()).Return(nil)

	token, err := s.replicationMessageHandler.MergeMessages(ctx, s.sourceCluster, lastMessageID, pageSize, pageToken)
	s.NoError(err)
	s.Equal(pageToken, token)
}
