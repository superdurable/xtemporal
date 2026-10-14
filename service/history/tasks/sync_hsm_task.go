package tasks

import (
	"time"

	enumsspb "github.com/superdurable/xtemporal/api/enums/v1"
	"github.com/superdurable/xtemporal/common/definition"
)

var _ Task = (*SyncHSMTask)(nil)

type (
	SyncHSMTask struct {
		definition.WorkflowKey
		VisibilityTimestamp time.Time
		TaskID              int64
		TargetClusters      []string
	}
)

func (a *SyncHSMTask) GetKey() Key {
	return NewImmediateKey(a.TaskID)
}

func (a *SyncHSMTask) GetTaskID() int64 {
	return a.TaskID
}

func (a *SyncHSMTask) SetTaskID(id int64) {
	a.TaskID = id
}

func (a *SyncHSMTask) GetVisibilityTime() time.Time {
	return a.VisibilityTimestamp
}

func (a *SyncHSMTask) SetVisibilityTime(timestamp time.Time) {
	a.VisibilityTimestamp = timestamp
}

func (a *SyncHSMTask) GetCategory() Category {
	return CategoryReplication
}

func (a *SyncHSMTask) GetType() enumsspb.TaskType {
	return enumsspb.TASK_TYPE_REPLICATION_SYNC_HSM
}
