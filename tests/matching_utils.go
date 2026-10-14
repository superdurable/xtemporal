package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	deploymentspb "github.com/superdurable/xtemporal/api/deployment/v1"
	"github.com/superdurable/xtemporal/api/matchingservice/v1"
	"github.com/superdurable/xtemporal/common/namespace"
	"github.com/superdurable/xtemporal/common/primitives/timestamp"
	"github.com/superdurable/xtemporal/common/testing/testvars"
	"github.com/superdurable/xtemporal/tests/testcore"
	enumspb "go.temporal.io/api/enums/v1"
)

// runWithMatchingBehaviors runs a test with all combinations of matching behaviors.
func runWithMatchingBehaviors(
	t *testing.T,
	baseOpts []testcore.TestOption,
	subtest func(s *testcore.TestEnv, behavior testcore.MatchingBehavior),
) {
	for _, behavior := range testcore.AllMatchingBehaviors() {
		t.Run(behavior.Name(), func(t *testing.T) {
			opts := append([]testcore.TestOption{}, baseOpts...)
			opts = append(opts, behavior.Options()...)

			env := testcore.NewEnv(t, opts...)
			behavior.InjectHooks(env)

			subtest(env, behavior)
		})
	}
}

// syncDeploymentVersionToTaskQueues sends a SyncDeploymentUserData request to the matching service
// to register a deployment version as current for the specified task queue types, then waits for
// the data to propagate to all partitions using CheckTaskQueueUserDataPropagation.
func syncDeploymentVersionToTaskQueues(
	t testing.TB,
	matchingClient matchingservice.MatchingServiceClient,
	namespaceID namespace.ID,
	tv *testvars.TestVars,
	tqTypes ...enumspb.TaskQueueType,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := timestamp.TimePtr(time.Now())
	resp, err := matchingClient.SyncDeploymentUserData(
		ctx, &matchingservice.SyncDeploymentUserDataRequest{
			NamespaceId:    namespaceID.String(),
			TaskQueue:      tv.TaskQueue().GetName(),
			TaskQueueTypes: tqTypes,
			Operation: &matchingservice.SyncDeploymentUserDataRequest_UpdateVersionData{
				UpdateVersionData: &deploymentspb.DeploymentVersionData{
					Version:           tv.DeploymentVersion(),
					RoutingUpdateTime: now,
					CurrentSinceTime:  now,
				},
			},
		},
	)
	require.NoError(t, err)

	// Wait for the data to propagate to all partitions.
	_, err = matchingClient.CheckTaskQueueUserDataPropagation(
		ctx, &matchingservice.CheckTaskQueueUserDataPropagationRequest{
			NamespaceId: namespaceID.String(),
			TaskQueue:   tv.TaskQueue().GetName(),
			Version:     resp.GetVersion(),
		},
	)
	require.NoError(t, err)
}
