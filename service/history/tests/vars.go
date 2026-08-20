package tests

import (
	"time"

	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/cluster"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/namespace"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/primitives/timestamp"
	"github.com/superdurable/temporal-server/service/history/configs"
	enumspb "go.temporal.io/api/enums/v1"
	namespacepb "go.temporal.io/api/namespace/v1"
)

var (
	Version = int64(1234)

	NamespaceID                              = namespace.ID("deadbeef-0123-4567-890a-bcdef0123456")
	Namespace                                = namespace.Name("mock namespace name")
	ParentNamespaceID                        = namespace.ID("deadbeef-0123-4567-890a-bcdef0123457")
	ParentNamespace                          = namespace.Name("mock parent namespace name")
	TargetNamespaceID                        = namespace.ID("deadbeef-0123-4567-890a-bcdef0123458")
	TargetNamespace                          = namespace.Name("mock target namespace name")
	ChildNamespaceID                         = namespace.ID("deadbeef-0123-4567-890a-bcdef0123459")
	ChildNamespace                           = namespace.Name("mock child namespace name")
	StandbyNamespaceID                       = namespace.ID("deadbeef-0123-4567-890a-bcdef0123460")
	StandbyNamespace                         = namespace.Name("mock standby namespace name")
	StandbyWithVisibilityArchivalNamespaceID = namespace.ID("deadbeef-0123-4567-890a-bcdef0123461")
	StandbyWithVisibilityArchivalNamespace   = namespace.Name("mock standby with visibility archival namespace name")
	MissedNamespaceID                        = namespace.ID("missed-namespace-id")
	MissedNamespace                          = namespace.Name("missed-namespace-name")
	WorkflowID                               = "mock-workflow-id"
	RunID                                    = "0d00698f-08e1-4d36-a3e2-3bf109f5d2d6"
	WorkflowKey                              = definition.NewWorkflowKey(NamespaceID.String(), WorkflowID, RunID)
	ArchetypeID                              = chasm.ArchetypeID(1234)

	LocalNamespaceEntry = namespace.NewLocalNamespaceForTest(
		&persistencespb.NamespaceInfo{Id: NamespaceID.String(), Name: Namespace.String()},
		&persistencespb.NamespaceConfig{
			Retention: timestamp.DurationFromDays(1),
			BadBinaries: &namespacepb.BadBinaries{
				Binaries: map[string]*namespacepb.BadBinaryInfo{
					"lololol": nil},
			},
		},
		cluster.TestCurrentClusterName,
	)

	GlobalNamespaceEntry = namespace.NewGlobalNamespaceForTest(
		&persistencespb.NamespaceInfo{Id: NamespaceID.String(), Name: Namespace.String()},
		&persistencespb.NamespaceConfig{
			Retention:               timestamp.DurationFromDays(1),
			VisibilityArchivalState: enumspb.ARCHIVAL_STATE_ENABLED,
			VisibilityArchivalUri:   "test:///visibility/archival",
		},
		&persistencespb.NamespaceReplicationConfig{
			ActiveClusterName: cluster.TestCurrentClusterName,
			Clusters: []string{
				cluster.TestCurrentClusterName,
				cluster.TestAlternativeClusterName,
			},
		},
		Version,
	)

	GlobalParentNamespaceEntry = namespace.NewGlobalNamespaceForTest(
		&persistencespb.NamespaceInfo{Id: ParentNamespaceID.String(), Name: ParentNamespace.String()},
		&persistencespb.NamespaceConfig{Retention: timestamp.DurationFromDays(1)},
		&persistencespb.NamespaceReplicationConfig{
			ActiveClusterName: cluster.TestCurrentClusterName,
			Clusters: []string{
				cluster.TestCurrentClusterName,
				cluster.TestAlternativeClusterName,
			},
		},
		Version,
	)

	GlobalTargetNamespaceEntry = namespace.NewGlobalNamespaceForTest(
		&persistencespb.NamespaceInfo{Id: TargetNamespaceID.String(), Name: TargetNamespace.String()},
		&persistencespb.NamespaceConfig{Retention: timestamp.DurationFromDays(1)},
		&persistencespb.NamespaceReplicationConfig{
			ActiveClusterName: cluster.TestCurrentClusterName,
			Clusters: []string{
				cluster.TestCurrentClusterName,
				cluster.TestAlternativeClusterName,
			},
		},
		Version,
	)

	GlobalStandbyNamespaceEntry = namespace.NewGlobalNamespaceForTest(
		&persistencespb.NamespaceInfo{Id: StandbyNamespaceID.String(), Name: StandbyNamespace.String()},
		&persistencespb.NamespaceConfig{Retention: timestamp.DurationFromDays(1)},
		&persistencespb.NamespaceReplicationConfig{
			ActiveClusterName: cluster.TestAlternativeClusterName,
			Clusters: []string{
				cluster.TestCurrentClusterName,
				cluster.TestAlternativeClusterName,
			},
		},
		Version,
	)

	GlobalStandbyWithVisibilityArchivalNamespaceEntry = namespace.NewGlobalNamespaceForTest(
		&persistencespb.NamespaceInfo{
			Id:   StandbyWithVisibilityArchivalNamespaceID.String(),
			Name: StandbyWithVisibilityArchivalNamespace.String(),
		},
		&persistencespb.NamespaceConfig{
			Retention:               timestamp.DurationFromDays(1),
			VisibilityArchivalState: enumspb.ARCHIVAL_STATE_ENABLED,
			VisibilityArchivalUri:   "test:///visibility/archival",
		},
		&persistencespb.NamespaceReplicationConfig{
			ActiveClusterName: cluster.TestAlternativeClusterName,
			Clusters: []string{
				cluster.TestCurrentClusterName,
				cluster.TestAlternativeClusterName,
			},
		},
		Version,
	)

	GlobalChildNamespaceEntry = namespace.NewGlobalNamespaceForTest(
		&persistencespb.NamespaceInfo{Id: ChildNamespaceID.String(), Name: ChildNamespace.String()},
		&persistencespb.NamespaceConfig{Retention: timestamp.DurationFromDays(1)},
		&persistencespb.NamespaceReplicationConfig{
			ActiveClusterName: cluster.TestCurrentClusterName,
			Clusters: []string{
				cluster.TestCurrentClusterName,
				cluster.TestAlternativeClusterName,
			},
		},
		Version,
	)

	CreateWorkflowExecutionResponse = &persistence.CreateWorkflowExecutionResponse{
		NewMutableStateStats: persistence.MutableStateStatistics{
			HistoryStatistics: &persistence.HistoryStatistics{},
		},
	}

	GetWorkflowExecutionResponse = &persistence.GetWorkflowExecutionResponse{
		MutableStateStats: persistence.MutableStateStatistics{},
	}

	UpdateWorkflowExecutionResponse = &persistence.UpdateWorkflowExecutionResponse{
		UpdateMutableStateStats: persistence.MutableStateStatistics{
			HistoryStatistics: &persistence.HistoryStatistics{},
		},
		NewMutableStateStats: nil,
	}
)

func NewDynamicConfig() *configs.Config {
	dc := dynamicconfig.NewNoopCollection()
	config := configs.NewConfig(dc, 1)
	config.EnableActivityEagerExecution = dynamicconfig.GetBoolPropertyFnFilteredByNamespace(true)
	config.NamespaceCacheRefreshInterval = dynamicconfig.GetDurationPropertyFn(time.Second)
	config.ReplicationEnableUpdateWithNewTaskMerge = dynamicconfig.GetBoolPropertyFn(true)
	config.EnableWorkflowIdReuseStartTimeValidation = dynamicconfig.GetBoolPropertyFnFilteredByNamespace(true)
	config.EnableTransitionHistory = dynamicconfig.GetBoolPropertyFnFilteredByNamespace(true)
	config.EnableChasm = dynamicconfig.GetBoolPropertyFnFilteredByNamespace(false)
	return config
}
