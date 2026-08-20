package listqueues_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/temporal-server/api/historyservice/v1"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/service/history/api/listqueues"
	"go.temporal.io/api/serviceerror"
)

// failingHistoryTaskQueueManager is a [persistence.HistoryTaskQueueManager] that always fails.
type failingHistoryTaskQueueManager struct {
	persistence.HistoryTaskQueueManager
}

func TestInvoke_UnavailableError(t *testing.T) {
	t.Parallel()

	_, err := listqueues.Invoke(
		context.Background(),
		failingHistoryTaskQueueManager{},
		&historyservice.ListQueuesRequest{
			QueueType: int32(persistence.QueueTypeHistoryDLQ),
			PageSize:  0,
		},
	)
	var unavailableErr *serviceerror.Unavailable
	require.ErrorAs(t, err, &unavailableErr)
	assert.ErrorContains(t, unavailableErr, "some random error")
}

func (m failingHistoryTaskQueueManager) ListQueues(
	context.Context,
	*persistence.ListQueuesRequest,
) (*persistence.ListQueuesResponse, error) {
	return nil, errors.New("some random error")
}
