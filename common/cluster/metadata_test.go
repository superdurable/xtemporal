package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/persistence"
	"go.uber.org/mock/gomock"
)

type (
	metadataSuite struct {
		suite.Suite
		*require.Assertions

		controller               *gomock.Controller
		mockClusterMetadataStore *persistence.MockClusterMetadataManager
		metadata                 *metadataImpl

		isGlobalNamespaceEnabled bool
		failoverVersionIncrement int64
		clusterName              string
		secondClusterName        string
		thirdClusterName         string
	}
)

func TestMetadataSuite(t *testing.T) {
	s := new(metadataSuite)
	suite.Run(t, s)
}

func (s *metadataSuite) SetupSuite() {
}

func (s *metadataSuite) TearDownSuite() {

}

func (s *metadataSuite) SetupTest() {
	s.Assertions = require.New(s.T())
	s.controller = gomock.NewController(s.T())
	s.mockClusterMetadataStore = persistence.NewMockClusterMetadataManager(s.controller)

	s.isGlobalNamespaceEnabled = true
	s.failoverVersionIncrement = 100
	s.clusterName = uuid.NewString()
	s.secondClusterName = uuid.NewString()
	s.thirdClusterName = uuid.NewString()

	clusterInfo := map[string]ClusterInformation{
		s.clusterName: {
			Enabled:                true,
			InitialFailoverVersion: int64(1),
			RPCAddress:             uuid.NewString(),
			ShardCount:             1,
			version:                1,
		},
		s.secondClusterName: {
			Enabled:                true,
			InitialFailoverVersion: int64(4),
			RPCAddress:             uuid.NewString(),
			ShardCount:             2,
			version:                1,
		},
		s.thirdClusterName: {
			Enabled:                true,
			InitialFailoverVersion: int64(5),
			RPCAddress:             uuid.NewString(),
			ShardCount:             1,
			version:                1,
		},
	}
	s.metadata = NewMetadata(
		s.isGlobalNamespaceEnabled,
		s.failoverVersionIncrement,
		s.clusterName,
		s.clusterName,
		clusterInfo,
		s.mockClusterMetadataStore,
		dynamicconfig.GetDurationPropertyFn(time.Second),
		log.NewNoopLogger(),
	).(*metadataImpl)
}

func (s *metadataSuite) TearDownTest() {
	s.controller.Finish()
}

func (s *metadataSuite) Test_Initialization() {
	s.Equal(s.isGlobalNamespaceEnabled, s.metadata.IsGlobalNamespaceEnabled())
	s.Equal(s.clusterName, s.metadata.GetMasterClusterName())
	s.Equal(s.clusterName, s.metadata.GetCurrentClusterName())
	s.True(s.metadata.IsMasterCluster())
	s.Equal(s.failoverVersionIncrement, s.metadata.GetFailoverVersionIncrement())
}

func (s *metadataSuite) Test_GetCurrentShardCount() {
	s.Equal(ShardCount{Count: 1, Version: 1}, s.metadata.GetCurrentShardCount())
}

func (s *metadataSuite) Test_RefreshClusterMetadata_ShardCountChange() {
	// A shard count bump is the only field that changes. It must still be
	// treated as a real update: picked up by GetCurrentShardCount and delivered
	// to change callbacks, otherwise history hosts never learn about a scale-up.
	all := s.metadata.GetAllClusterInfo()

	// Echo every cluster back unchanged except the current one's shard count, so
	// the absent-cluster removal path cannot mask the result.
	records := make([]*persistence.GetClusterMetadataResponse, 0, len(all))
	for name, info := range all {
		shardCount, version := info.ShardCount, info.version
		if name == s.clusterName {
			shardCount, version = 2, 2
		}
		records = append(records, &persistence.GetClusterMetadataResponse{
			ClusterMetadata: &persistencespb.ClusterMetadata{
				ClusterName:            name,
				IsConnectionEnabled:    info.Enabled,
				InitialFailoverVersion: info.InitialFailoverVersion,
				HistoryShardCount:      shardCount,
				ClusterAddress:         info.RPCAddress,
			},
			Version: version,
		})
	}

	var gotOld, gotNew *ClusterInformation
	callbackFired := false
	s.metadata.clusterChangeCallback[uuid.NewString()] = func(old, new map[string]*ClusterInformation) {
		callbackFired = true
		gotOld, gotNew = old[s.clusterName], new[s.clusterName]
	}

	s.mockClusterMetadataStore.EXPECT().ListClusterMetadata(gomock.Any(), gomock.Any()).Return(
		&persistence.ListClusterMetadataResponse{ClusterMetadata: records}, nil)

	s.NoError(s.metadata.refreshClusterMetadata(context.Background()))
	s.True(callbackFired, "shard count change must fire a metadata change callback")
	s.Require().NotNil(gotOld, "current cluster missing from callback entries")
	s.Require().NotNil(gotNew, "current cluster missing from callback entries")
	s.Equal(int32(1), gotOld.ShardCount)
	s.Equal(int32(2), gotNew.ShardCount)
	s.Equal(ShardCount{Count: 2, Version: 2}, s.metadata.GetCurrentShardCount())
}

func (s *metadataSuite) Test_GetNextFailoverVersion() {
	currentVersion := int64(102)
	s.Equal(currentVersion+s.failoverVersionIncrement-1, s.metadata.GetNextFailoverVersion(s.clusterName, currentVersion))
}

func (s *metadataSuite) Test_IsVersionFromSameCluster() {
	s.True(s.metadata.IsVersionFromSameCluster(101, 1001))
	s.False(s.metadata.IsVersionFromSameCluster(101, 103))
}

func (s *metadataSuite) Test_ClusterNameForFailoverVersion() {
	clusterName := s.metadata.ClusterNameForFailoverVersion(true, 101)
	s.Equal(s.clusterName, clusterName)

	clusterName2 := s.metadata.ClusterNameForFailoverVersion(true, 204)
	s.Equal(s.secondClusterName, clusterName2)

	clusterName3 := s.metadata.ClusterNameForFailoverVersion(true, 217)
	s.Equal(unknownClusterNamePrefix+"17", clusterName3)
}

func (s *metadataSuite) Test_RegisterMetadataChangeCallback() {
	s.metadata.RegisterMetadataChangeCallback(
		s,
		func(oldClusterMetadata map[string]*ClusterInformation, newClusterMetadata map[string]*ClusterInformation) {
			s.Equal(3, len(newClusterMetadata))
		})

	s.metadata.UnRegisterMetadataChangeCallback(s)
	s.Equal(0, len(s.metadata.clusterChangeCallback))
}

func (s *metadataSuite) Test_RefreshClusterMetadata_Success() {
	id := uuid.NewString()
	s.metadata.clusterChangeCallback[id] = func(oldClusterMetadata map[string]*ClusterInformation, newClusterMetadata map[string]*ClusterInformation) {
		oldMetadata, ok := oldClusterMetadata[id]
		s.True(ok)
		s.Nil(oldMetadata)
		newMetadata, ok := newClusterMetadata[id]
		s.True(ok)
		s.NotNil(newMetadata)

		oldMetadata, ok = oldClusterMetadata[s.secondClusterName]
		s.True(ok)
		s.NotNil(oldMetadata)
		newMetadata, ok = newClusterMetadata[s.secondClusterName]
		s.True(ok)
		s.Nil(newMetadata)

		oldMetadata, ok = oldClusterMetadata[s.thirdClusterName]
		s.True(ok)
		s.NotNil(oldMetadata)
		newMetadata, ok = newClusterMetadata[s.thirdClusterName]
		s.True(ok)
		s.NotNil(newMetadata)
	}

	s.mockClusterMetadataStore.EXPECT().ListClusterMetadata(gomock.Any(), gomock.Any()).Return(
		&persistence.ListClusterMetadataResponse{
			ClusterMetadata: []*persistence.GetClusterMetadataResponse{
				{
					// No change and not include in callback
					ClusterMetadata: &persistencespb.ClusterMetadata{
						ClusterName:            s.clusterName,
						IsConnectionEnabled:    true,
						InitialFailoverVersion: 1,
						HistoryShardCount:      1,
						ClusterAddress:         uuid.NewString(),
						HttpAddress:            uuid.NewString(),
					},
					Version: 1,
				},
				{
					// Updated, included in callback
					ClusterMetadata: &persistencespb.ClusterMetadata{
						ClusterName:            s.thirdClusterName,
						IsConnectionEnabled:    true,
						InitialFailoverVersion: 1,
						HistoryShardCount:      1,
						ClusterAddress:         uuid.NewString(),
						HttpAddress:            uuid.NewString(),
						Tags:                   map[string]string{"test": "test"},
					},
					Version: 2,
				},
				{
					// Newly added, included in callback
					ClusterMetadata: &persistencespb.ClusterMetadata{
						ClusterName:            id,
						IsConnectionEnabled:    true,
						InitialFailoverVersion: 2,
						HistoryShardCount:      2,
						ClusterAddress:         uuid.NewString(),
						HttpAddress:            uuid.NewString(),
						Tags:                   map[string]string{"test": "test"},
					},
					Version: 2,
				},
			},
		}, nil)
	err := s.metadata.refreshClusterMetadata(context.Background())
	s.NoError(err)
	clusterInfo := s.metadata.GetAllClusterInfo()
	s.Equal("test", clusterInfo[s.thirdClusterName].Tags["test"])
	s.Equal("test", clusterInfo[id].Tags["test"])
}

func (s *metadataSuite) Test_ListAllClusterMetadataFromDB_Success() {
	nextPageSizeToken := []byte{1}
	newClusterName := uuid.NewString()
	s.mockClusterMetadataStore.EXPECT().ListClusterMetadata(gomock.Any(), &persistence.ListClusterMetadataRequest{
		PageSize:      defaultClusterMetadataPageSize,
		NextPageToken: nil,
	}).Return(
		&persistence.ListClusterMetadataResponse{
			ClusterMetadata: []*persistence.GetClusterMetadataResponse{
				{
					ClusterMetadata: &persistencespb.ClusterMetadata{
						ClusterName:            s.clusterName,
						IsConnectionEnabled:    true,
						InitialFailoverVersion: 1,
						HistoryShardCount:      1,
						ClusterAddress:         uuid.NewString(),
						HttpAddress:            uuid.NewString(),
					},
					Version: 1,
				},
			},
			NextPageToken: nextPageSizeToken,
		}, nil).Times(1)
	s.mockClusterMetadataStore.EXPECT().ListClusterMetadata(gomock.Any(), &persistence.ListClusterMetadataRequest{
		PageSize:      defaultClusterMetadataPageSize,
		NextPageToken: nextPageSizeToken,
	}).Return(
		&persistence.ListClusterMetadataResponse{
			ClusterMetadata: []*persistence.GetClusterMetadataResponse{
				{
					ClusterMetadata: &persistencespb.ClusterMetadata{
						ClusterName:            newClusterName,
						IsConnectionEnabled:    true,
						InitialFailoverVersion: 2,
						HistoryShardCount:      2,
						ClusterAddress:         uuid.NewString(),
						HttpAddress:            uuid.NewString(),
					},
					Version: 2,
				},
			},
		}, nil).Times(1)

	resp, err := s.metadata.listAllClusterMetadataFromDB(context.Background())
	s.NoError(err)
	s.Equal(2, len(resp))
}
