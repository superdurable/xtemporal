package workflow

import (
	"context"
	"time"

	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/chasm"
	historyi "github.com/superdurable/temporal-server/service/history/interfaces"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"go.temporal.io/api/serviceerror"
)

var _ historyi.ChasmTree = (*noopChasmTree)(nil)

var NoopChasmTree = &noopChasmTree{}

type noopChasmTree struct{}

func (*noopChasmTree) CloseTransaction() (chasm.NodesMutation, error) {
	return chasm.NodesMutation{}, nil
}

func (*noopChasmTree) Snapshot(*persistencespb.VersionedTransition) chasm.NodesSnapshot {
	return chasm.NodesSnapshot{}
}

func (*noopChasmTree) ApplySystemMutation(chasm.NodesMutation) error {
	return nil
}

func (*noopChasmTree) ApplyMutation(chasm.NodesMutation) error {
	return nil
}

func (*noopChasmTree) ApplySnapshot(chasm.NodesSnapshot) error {
	return nil
}

func (*noopChasmTree) RefreshTasks() error {
	return nil
}

func (*noopChasmTree) IsStateDirty() bool {
	return false
}

func (*noopChasmTree) IsDirty() bool {
	return false
}

func (*noopChasmTree) Terminate(chasm.TerminateComponentRequest) error {
	return nil
}

func (*noopChasmTree) Archetype() (chasm.Archetype, error) {
	return chasm.WorkflowArchetype, nil
}

func (*noopChasmTree) ArchetypeID() chasm.ArchetypeID {
	return chasm.WorkflowArchetypeID
}

func (*noopChasmTree) EachPureTask(
	deadline time.Time,
	callback func(executor chasm.NodePureTask, taskAttributes chasm.TaskAttributes, task any) (bool, error),
) error {
	return nil
}

func (*noopChasmTree) IsStale(chasm.ComponentRef) error {
	return nil
}

func (*noopChasmTree) Component(chasm.Context, chasm.ComponentRef) (chasm.Component, error) {
	return nil, serviceerror.NewInternal("Component() method invoked on noop CHASM tree")
}

func (*noopChasmTree) ComponentByPath(chasm.Context, []string) (chasm.Component, error) {
	return nil, serviceerror.NewInternal("ComponentByPath() method invoked on noop CHASM tree")
}

func (*noopChasmTree) ExecuteSideEffectTask(
	ctx context.Context,
	executionKey chasm.ExecutionKey,
	task *tasks.ChasmTask,
	validate func(chasm.NodeBackend, chasm.Context, chasm.Component) error,
) error {
	return nil
}

func (*noopChasmTree) ExecuteSideEffectDiscardTask(
	ctx context.Context,
	executionKey chasm.ExecutionKey,
	task *tasks.ChasmTask,
	validate func(chasm.NodeBackend, chasm.Context, chasm.Component) error,
) error {
	return nil
}

func (*noopChasmTree) ValidateSideEffectTask(
	ctx context.Context,
	task *tasks.ChasmTask,
) (bool, error) {
	return false, nil
}
