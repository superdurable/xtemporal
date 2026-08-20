package tasks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	enumsspb "github.com/superdurable/temporal-server/api/enums/v1"
	"github.com/superdurable/temporal-server/common/definition"
)

func TestArchiveExecutionTask(t *testing.T) {
	workflowKey := definition.NewWorkflowKey("namespace", "workflowID", "runID")
	visibilityTimestamp := time.Now()
	taskID := int64(123)
	version := int64(456)
	task := &ArchiveExecutionTask{
		WorkflowKey:         workflowKey,
		VisibilityTimestamp: visibilityTimestamp,
		TaskID:              taskID,
		Version:             version,
	}
	assert.Equal(t, NewKey(visibilityTimestamp, taskID), task.GetKey())
	assert.Equal(t, taskID, task.GetTaskID())
	assert.Equal(t, visibilityTimestamp, task.GetVisibilityTime())
	assert.Equal(t, version, task.GetVersion())
	assert.Equal(t, CategoryArchival, task.GetCategory())
	assert.Equal(t, enumsspb.TASK_TYPE_ARCHIVAL_ARCHIVE_EXECUTION, task.GetType())
}
