package sql

import (
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
)

type taskQueuePageToken struct {
	MinRangeHash   uint32
	MinTaskQueueId []byte
}

type matchingTaskPageToken struct {
	TaskPass int64 `json:",omitempty"`
	TaskID   int64
}

type userDataListNextPageToken struct {
	LastTaskQueueName string
}

// newTaskPersistence creates a new instance of TaskStore
func newTaskPersistence(
	db sqlplugin.DB,
	taskScanPartitions int,
	logger log.Logger,
	enableFairness bool,
	serializer serialization.Serializer,
) (persistence.TaskStore, error) {
	store := SqlStore{
		DB:         db,
		logger:     logger,
		serializer: serializer,
	}
	userDataStore := userDataStore{SqlStore: store}
	taskQueueStore := taskQueueStore{
		SqlStore:           store,
		version:            sqlplugin.MatchingTaskVersion1,
		taskScanPartitions: uint32(taskScanPartitions),
	}
	if enableFairness {
		taskQueueStore.version = sqlplugin.MatchingTaskVersion2
		return newTaskManagerV2(db, userDataStore, taskQueueStore, logger, serializer)
	}
	return newTaskManagerV1(db, userDataStore, taskQueueStore, logger, serializer)
}
