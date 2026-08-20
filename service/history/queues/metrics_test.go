package queues

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	"github.com/superdurable/temporal-server/service/history/tasks"
)

func TestGetArchivalTaskTypeTagValue(t *testing.T) {
	assert.Equal(t, "ArchivalTaskArchiveExecution", GetArchivalTaskTypeTagValue(&tasks.ArchiveExecutionTask{}))

	unknownTask := &tasks.CloseExecutionTask{}
	assert.Equal(t, unknownTask.GetType().String(), GetArchivalTaskTypeTagValue(unknownTask))
}

func TestGetArchetypeTag(t *testing.T) {
	registry := chasm.NewRegistry(log.NewTestLogger())

	t.Run("legacy task without HasArchetypeID defaults to workflow", func(t *testing.T) {
		task := &tasks.ActivityTask{}
		tag := getArchetypeTag(task, registry)
		assert.Equal(t, metrics.ArchetypeTag(chasm.WorkflowComponentName), tag)
	})

	t.Run("HasArchetypeID task with unregistered ID defaults to workflow", func(t *testing.T) {
		task := &tasks.ChasmTaskPure{ArchetypeID: 9999}
		tag := getArchetypeTag(task, registry)
		assert.Equal(t, metrics.ArchetypeTag(chasm.WorkflowComponentName), tag)
	})
}
