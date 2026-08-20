package tasks

import (
	"time"

	enumsspb "github.com/superdurable/temporal-server/api/enums/v1"
	"github.com/superdurable/temporal-server/common/definition"
)

var _ Task = (*UpsertExecutionVisibilityTask)(nil)

type (
	UpsertExecutionVisibilityTask struct {
		definition.WorkflowKey
		VisibilityTimestamp time.Time
		TaskID              int64
	}
)

func (t *UpsertExecutionVisibilityTask) GetKey() Key {
	return NewImmediateKey(t.TaskID)
}

func (t *UpsertExecutionVisibilityTask) GetTaskID() int64 {
	return t.TaskID
}

func (t *UpsertExecutionVisibilityTask) SetTaskID(id int64) {
	t.TaskID = id
}

func (t *UpsertExecutionVisibilityTask) GetVisibilityTime() time.Time {
	return t.VisibilityTimestamp
}

func (t *UpsertExecutionVisibilityTask) SetVisibilityTime(timestamp time.Time) {
	t.VisibilityTimestamp = timestamp
}

func (t *UpsertExecutionVisibilityTask) GetCategory() Category {
	return CategoryVisibility
}

func (t *UpsertExecutionVisibilityTask) GetType() enumsspb.TaskType {
	return enumsspb.TASK_TYPE_VISIBILITY_UPSERT_EXECUTION
}
