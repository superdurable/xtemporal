package deletedlqtasks_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonspb "github.com/superdurable/temporal-server/api/common/v1"
	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/persistencetest"
	"github.com/superdurable/temporal-server/service/history/api/deletedlqtasks"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"go.temporal.io/api/serviceerror"
	"google.golang.org/grpc/codes"
)

func TestInvoke_InvalidCategory(t *testing.T) {
	t.Parallel()

	queueKey := persistencetest.GetQueueKey(t, persistencetest.WithQueueType(persistence.QueueTypeHistoryDLQ))
	_, err := deletedlqtasks.Invoke(context.Background(), nil, &historyservice.DeleteDLQTasksRequest{
		DlqKey: &commonspb.HistoryDLQKey{
			TaskCategory:  -1,
			SourceCluster: queueKey.SourceCluster,
			TargetCluster: queueKey.TargetCluster,
		},
	}, tasks.NewDefaultTaskCategoryRegistry())
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, serviceerror.ToStatus(err).Code())
	assert.ErrorContains(t, err, "-1")
}

func TestInvoke_ErrDeleteMissingMessageIDUpperBound(t *testing.T) {
	t.Parallel()

	queueKey := persistencetest.GetQueueKey(t, persistencetest.WithQueueType(persistence.QueueTypeHistoryDLQ))
	_, err := deletedlqtasks.Invoke(context.Background(), nil, &historyservice.DeleteDLQTasksRequest{
		DlqKey: &commonspb.HistoryDLQKey{
			TaskCategory:  int32(queueKey.Category.ID()),
			SourceCluster: queueKey.SourceCluster,
			TargetCluster: queueKey.TargetCluster,
		},
	}, tasks.NewDefaultTaskCategoryRegistry())
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, serviceerror.ToStatus(err).Code())
	assert.ErrorContains(t, err, "inclusive_max_task_metadata")
}
