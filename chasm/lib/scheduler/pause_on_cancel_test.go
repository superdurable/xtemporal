package scheduler_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	persistencespb "github.com/superdurable/xtemporal/api/persistence/v1"
	schedulespb "github.com/superdurable/xtemporal/api/schedule/v1"
	failurepb "go.temporal.io/api/failure/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCanceledWorkflowDoesNotPauseByDefault(t *testing.T) {
	sched, ctx, _ := setupSchedulerForTest(t)
	sched.Schedule.Policies.PauseOnFailure = true
	invoker := sched.Invoker.Get(ctx)
	invoker.BufferedStarts = []*schedulespb.BufferedStart{{
		RequestId:  "request",
		WorkflowId: "workflow",
		RunId:      "run",
		Attempt:    1,
	}}

	err := sched.HandleNexusCompletion(ctx, &persistencespb.ChasmNexusCompletion{
		RequestId: "request",
		Outcome: &persistencespb.ChasmNexusCompletion_Failure{
			Failure: &failurepb.Failure{
				FailureInfo: &failurepb.Failure_CanceledFailureInfo{
					CanceledFailureInfo: &failurepb.CanceledFailureInfo{},
				},
			},
		},
		CloseTime: timestamppb.New(time.Now()),
	})
	require.NoError(t, err)
	require.False(t, sched.Schedule.State.Paused,
		"CanceledTerminatedCountAsFailures defaults to false, so cancellation must not pause")
}
