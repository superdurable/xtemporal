package tasks

import (
	"time"

	enumsspb "github.com/superdurable/temporal-server/api/enums/v1"
	"github.com/superdurable/temporal-server/common"
	"github.com/superdurable/temporal-server/common/definition"
)

type (
	FakeTask struct {
		definition.WorkflowKey
		VisibilityTimestamp time.Time
		TaskID              int64
		Version             int64
		Category            Category
	}
)

func NewFakeTask(
	workflowKey definition.WorkflowKey,
	category Category,
	visibilityTimestamp time.Time,
) Task {
	return &FakeTask{
		WorkflowKey:         workflowKey,
		TaskID:              common.EmptyEventTaskID,
		Version:             common.EmptyVersion,
		VisibilityTimestamp: visibilityTimestamp,
		Category:            category,
	}
}

func (f *FakeTask) GetKey() Key {
	if f.Category.Type() == CategoryTypeImmediate {
		return NewImmediateKey(f.TaskID)
	}
	return NewKey(f.VisibilityTimestamp, f.TaskID)
}

func (f *FakeTask) GetVersion() int64 {
	return f.Version
}

func (f *FakeTask) SetVersion(version int64) {
	f.Version = version
}

func (f *FakeTask) GetTaskID() int64 {
	return f.TaskID
}

func (f *FakeTask) SetTaskID(id int64) {
	f.TaskID = id
}

func (f *FakeTask) GetVisibilityTime() time.Time {
	return f.VisibilityTimestamp
}

func (f *FakeTask) SetVisibilityTime(t time.Time) {
	f.VisibilityTimestamp = t
}

func (f *FakeTask) GetCategory() Category {
	return f.Category
}

func (f *FakeTask) GetType() enumsspb.TaskType {
	return enumsspb.TASK_TYPE_UNSPECIFIED
}
