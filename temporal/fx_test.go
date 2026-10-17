package temporal

import (
	"context"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	persistencespb "github.com/superdurable/xtemporal/api/persistence/v1"
	"github.com/superdurable/xtemporal/common/archiver"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/service/history/tasks"
	"github.com/superdurable/xtemporal/tests/testutils"
	enumspb "go.temporal.io/api/enums/v1"
	"go.uber.org/mock/gomock"
)

func TestInitCurrentClusterMetadataRecord(t *testing.T) {
	configDir := path.Join(testutils.GetRepoRootDirectory(), "config")
	cfg, err := config.Load(
		config.WithEnv("development-cass-es"),
		config.WithConfigDir(configDir),
	)
	require.NoError(t, err)
	controller := gomock.NewController(t)

	mockClusterMetadataManager := persistence.NewMockClusterMetadataManager(controller)
	mockClusterMetadataManager.EXPECT().SaveClusterMetadata(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request *persistence.SaveClusterMetadataRequest) (bool, error) {
			require.Equal(t, cfg.ClusterMetadata.EnableGlobalNamespace, request.IsGlobalNamespaceEnabled)
			require.Equal(t, cfg.ClusterMetadata.CurrentClusterName, request.ClusterName)
			require.Equal(t, cfg.ClusterMetadata.ClusterInformation[cfg.ClusterMetadata.CurrentClusterName].RPCAddress, request.ClusterAddress)
			require.Equal(t, cfg.ClusterMetadata.ClusterInformation[cfg.ClusterMetadata.CurrentClusterName].HTTPAddress, request.HttpAddress)
			require.Equal(t, cfg.ClusterMetadata.ClusterInformation[cfg.ClusterMetadata.CurrentClusterName].InitialFailoverVersion, request.InitialFailoverVersion)
			require.Equal(t, cfg.Persistence.NumHistoryShards, request.HistoryShardCount)
			require.Equal(t, cfg.ClusterMetadata.FailoverVersionIncrement, request.FailoverVersionIncrement)
			require.Equal(t, int64(0), request.Version)
			return true, nil
		},
	)
	err = initCurrentClusterMetadataRecord(
		context.TODO(),
		mockClusterMetadataManager,
		cfg,
		nil,
		log.NewNoopLogger(),
	)
	require.NoError(t, err)
}

func TestUpdateCurrentClusterMetadataRecord(t *testing.T) {
	configDir := path.Join(testutils.GetRepoRootDirectory(), "config")
	cfg, err := config.Load(
		config.WithEnv("development-cluster-a"),
		config.WithConfigDir(configDir),
	)
	require.NoError(t, err)
	controller := gomock.NewController(t)

	mockClusterMetadataManager := persistence.NewMockClusterMetadataManager(controller)
	mockClusterMetadataManager.EXPECT().SaveClusterMetadata(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request *persistence.SaveClusterMetadataRequest) (bool, error) {
			require.Equal(t, cfg.ClusterMetadata.EnableGlobalNamespace, request.IsGlobalNamespaceEnabled)
			require.Empty(t, request.ClusterName)
			require.Equal(t, cfg.ClusterMetadata.ClusterInformation[cfg.ClusterMetadata.CurrentClusterName].RPCAddress, request.ClusterAddress)
			require.Equal(t, cfg.ClusterMetadata.ClusterInformation[cfg.ClusterMetadata.CurrentClusterName].HTTPAddress, request.HttpAddress)
			require.Equal(t, cfg.ClusterMetadata.ClusterInformation[cfg.ClusterMetadata.CurrentClusterName].InitialFailoverVersion, request.InitialFailoverVersion)
			require.Equal(t, int32(0), request.HistoryShardCount)
			require.Equal(t, cfg.ClusterMetadata.FailoverVersionIncrement, request.FailoverVersionIncrement)
			require.Equal(t, int64(1), request.Version)
			return true, nil
		},
	)
	updateRecord := &persistence.GetClusterMetadataResponse{
		ClusterMetadata: &persistencespb.ClusterMetadata{},
		Version:         1,
	}
	err = updateCurrentClusterMetadataRecord(
		context.TODO(),
		mockClusterMetadataManager,
		cfg,
		nil,
		updateRecord,
	)
	require.NoError(t, err)
}

func TestOverwriteCurrentClusterMetadataWithDBRecord(t *testing.T) {
	configDir := path.Join(testutils.GetRepoRootDirectory(), "config")
	cfg, err := config.Load(
		config.WithEnv("development-cass-es"),
		config.WithConfigDir(configDir),
	)
	require.NoError(t, err)

	dbRecord := &persistence.GetClusterMetadataResponse{
		ClusterMetadata: &persistencespb.ClusterMetadata{
			HistoryShardCount:        1024,
			FailoverVersionIncrement: 10000,
			IsGlobalNamespaceEnabled: true,
		},
		Version: 1,
	}
	overwriteCurrentClusterMetadataWithDBRecord(
		cfg,
		dbRecord,
		log.NewNoopLogger(),
	)
	require.Equal(t, int64(10000), cfg.ClusterMetadata.FailoverVersionIncrement)
	require.True(t, cfg.ClusterMetadata.EnableGlobalNamespace)
	require.Equal(t, int32(1024), cfg.Persistence.NumHistoryShards)
}

func TestUpdateIndexSearchAttributes(t *testing.T) {
	testCases := []struct {
		name        string
		initialISA  map[string]*persistencespb.IndexSearchAttributes
		cmISA       map[string]*persistencespb.IndexSearchAttributes
		expectedISA map[string]*persistencespb.IndexSearchAttributes
		out         bool
	}{
		{
			name:        "noop nil initial isa",
			initialISA:  nil,
			cmISA:       nil,
			expectedISA: nil,
			out:         false,
		},
		{
			name:        "noop empty initial isa",
			initialISA:  map[string]*persistencespb.IndexSearchAttributes{},
			cmISA:       nil,
			expectedISA: nil,
			out:         false,
		},
		{
			name: "noop no changes",
			initialISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			cmISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			expectedISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			out: false,
		},
		{
			name: "noop initial is subset",
			initialISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			cmISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
						"Int02":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
						"Int02":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			expectedISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
						"Int02":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
						"Int02":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			out: false,
		},
		{
			name: "cm is empty",
			initialISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			cmISA: nil,
			expectedISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			out: true,
		},
		{
			name: "initial is superset",
			initialISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			cmISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
					},
				},
			},
			expectedISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			out: true,
		},
		{
			name: "index is missing",
			initialISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			cmISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			expectedISA: map[string]*persistencespb.IndexSearchAttributes{
				"my-index-1": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Keyword02": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
				"my-index-2": &persistencespb.IndexSearchAttributes{
					CustomSearchAttributes: map[string]enumspb.IndexedValueType{
						"Keyword01": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
						"Int01":     enumspb.INDEXED_VALUE_TYPE_INT,
					},
				},
			},
			out: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cm := &persistence.GetClusterMetadataResponse{
				ClusterMetadata: &persistencespb.ClusterMetadata{
					IndexSearchAttributes: tc.cmISA,
				},
			}
			out := updateIndexSearchAttributes(tc.initialISA, cm)
			require.Equal(t, tc.out, out)
			require.Equal(t, tc.expectedISA, cm.IndexSearchAttributes)
		})
	}
}

// xtemporal: Dex indexes begin without preallocated SQL search attribute slots.
func TestParadeDBInitialIndexSearchAttributes(t *testing.T) {
	stores := []config.DataStore{
		{CustomDataStoreConfig: &config.CustomDatastoreConfig{Name: "paradedb-dex", IndexName: "dex_visibility"}},
		{SQL: &config.SQL{DatabaseName: "sql_visibility"}},
	}
	got := initialVisibilityIndexSearchAttributes(stores, nil)
	require.Empty(t, got["dex_visibility"].CustomSearchAttributes)
	require.Contains(t, got["sql_visibility"].CustomSearchAttributes, "Keyword01")
}

func TestTaskCategoryRegistryProvider(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		historyState           archiver.ArchivalState
		visibilityState        archiver.ArchivalState
		expectArchivalCategory bool
	}{
		{
			name:                   "both disabled",
			historyState:           archiver.ArchivalDisabled,
			visibilityState:        archiver.ArchivalDisabled,
			expectArchivalCategory: false,
		},
		{
			name:                   "history enabled",
			historyState:           archiver.ArchivalEnabled,
			visibilityState:        archiver.ArchivalDisabled,
			expectArchivalCategory: true,
		},
		{
			name:                   "visibility enabled",
			historyState:           archiver.ArchivalDisabled,
			visibilityState:        archiver.ArchivalEnabled,
			expectArchivalCategory: true,
		},
		{
			name:                   "both enabled",
			historyState:           archiver.ArchivalEnabled,
			visibilityState:        archiver.ArchivalEnabled,
			expectArchivalCategory: true,
		},
		{
			name:                   "callbacks enabled, archival disabled",
			historyState:           archiver.ArchivalDisabled,
			visibilityState:        archiver.ArchivalDisabled,
			expectArchivalCategory: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			archivalMetadata := archiver.NewMockArchivalMetadata(ctrl)
			historyArchivalConfig := archiver.NewMockArchivalConfig(ctrl)
			historyArchivalConfig.EXPECT().StaticClusterState().Return(tc.historyState).AnyTimes()
			archivalMetadata.EXPECT().GetHistoryConfig().Return(historyArchivalConfig).AnyTimes()
			visibilityArchivalConfig := archiver.NewMockArchivalConfig(ctrl)
			visibilityArchivalConfig.EXPECT().StaticClusterState().Return(tc.visibilityState).AnyTimes()
			archivalMetadata.EXPECT().GetVisibilityConfig().Return(visibilityArchivalConfig).AnyTimes()
			registry := TaskCategoryRegistryProvider(archivalMetadata)
			_, ok := registry.GetCategoryByID(tasks.CategoryIDArchival)
			if tc.expectArchivalCategory {
				require.True(t, ok)
			} else {
				require.False(t, ok)
			}
		})
	}
}
