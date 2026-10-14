package tdbgtest_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.temporal.io/api/workflowservice/v1"
	"github.com/superdurable/xtemporal/api/adminservice/v1"
	commonspb "github.com/superdurable/xtemporal/api/common/v1"
	persistencespb "github.com/superdurable/xtemporal/api/persistence/v1"
	"github.com/superdurable/xtemporal/common/definition"
	"github.com/superdurable/xtemporal/common/persistence/serialization"
	"github.com/superdurable/xtemporal/service/history/tasks"
	"github.com/superdurable/xtemporal/service/history/tests"
	"github.com/superdurable/xtemporal/tools/tdbg"
	"github.com/superdurable/xtemporal/tools/tdbg/tdbgtest"
	"google.golang.org/grpc"
)

type (
	testClient struct {
		adminservice.AdminServiceClient
		getDLQTasksFn func(request *adminservice.GetDLQTasksRequest) (*adminservice.GetDLQTasksResponse, error)
	}
)

// TestParseDLQMessages tests that [tdbgtest.ParseDLQMessages] can parse a JSONL file containing serialized tasks.
func TestParseDLQMessages(t *testing.T) {
	t.Parallel()

	task := &tasks.WorkflowTask{
		WorkflowKey: definition.WorkflowKey{
			WorkflowID: tests.WorkflowID,
		},
		TaskID: 13,
	}
	taskSerializer := serialization.NewSerializer()
	blob, err := taskSerializer.SerializeTask(task)
	require.NoError(t, err)
	client := &testClient{
		getDLQTasksFn: func(request *adminservice.GetDLQTasksRequest) (*adminservice.GetDLQTasksResponse, error) {
			return &adminservice.GetDLQTasksResponse{
				DlqTasks: []*commonspb.HistoryDLQTask{{
					Metadata: &commonspb.HistoryDLQTaskMetadata{
						MessageId: 21,
					},
					Payload: &commonspb.HistoryTask{
						ShardId: 34,
						Blob:    blob,
					},
				},
				},
			}, nil
		},
	}
	var b bytes.Buffer
	app := tdbgtest.NewCliApp(func(params *tdbg.Params) {
		params.ClientFactory = client
		params.Writer = &b
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	err = app.RunContext(ctx, []string{
		"tdbg",
		"--" + tdbg.FlagYes,
		"dlq",
		"read",
		"--" + tdbg.FlagDLQType, "1",
		"--" + tdbg.FlagTargetCluster, "test-target-cluster",
	})
	require.NoError(t, err)
	output := b.String()
	t.Log("output:", output)
	messages, err := tdbgtest.ParseDLQMessages(&b, func() *persistencespb.TransferTaskInfo {
		return new(persistencespb.TransferTaskInfo)
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	message := messages[0]
	assert.Equal(t, 13, int(message.Payload.TaskId))
	assert.Equal(t, 21, int(message.MessageID))
	assert.Equal(t, 34, int(message.ShardID))
}

func (t *testClient) AdminClient(*cli.Context) adminservice.AdminServiceClient {
	return t
}

func (t *testClient) WorkflowClient(*cli.Context) workflowservice.WorkflowServiceClient {
	panic("unimplemented")
}

func (t *testClient) GetDLQTasks(
	_ context.Context,
	request *adminservice.GetDLQTasksRequest,
	_ ...grpc.CallOption,
) (*adminservice.GetDLQTasksResponse, error) {
	return t.getDLQTasksFn(request)
}
