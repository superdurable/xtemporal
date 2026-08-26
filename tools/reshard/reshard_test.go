package reshard_test

import (
	"context"
	dbsql "database/sql"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/definition"
	"github.com/superdurable/temporal-server/common/dynamicconfig"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/metrics"
	p "github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"github.com/superdurable/temporal-server/common/persistence/sql"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin"
	_ "github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin/sqlite"
	"github.com/superdurable/temporal-server/common/resolver"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"github.com/superdurable/temporal-server/tools/reshard"
)

const (
	oldShardCount = 4
	newShardCount = 8
)

type testEnv struct {
	executionManager p.ExecutionManager
	shardManager     p.ShardManager
	taskStore        reshard.TaskStore
	registry         tasks.TaskCategoryRegistry
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	cfg := &config.SQL{
		PluginName:        "sqlite",
		DatabaseName:      uuid.NewString(),
		ConnectAttributes: map[string]string{"mode": "memory", "cache": "private"},
	}
	logger := log.NewNoopLogger()

	adminDB, err := sql.NewSQLAdminDB(sqlplugin.DbKindUnknown, cfg, resolver.NewNoopResolver(), logger, metrics.NoopMetricsHandler)
	require.NoError(t, err)
	require.NoError(t, adminDB.CreateDatabase(cfg.DatabaseName))

	// The sqlite plugin materialises the full schema when the in-memory database is created.
	t.Cleanup(func() { _ = adminDB.Close() })

	factory := sql.NewFactory(*cfg, resolver.NewNoopResolver(), "test", logger, metrics.NoopMetricsHandler, serialization.NewSerializer())
	shardStore, err := factory.NewShardStore()
	require.NoError(t, err)
	executionStore, err := factory.NewExecutionStore()
	require.NoError(t, err)
	t.Cleanup(factory.Close)

	taskStore, err := sql.NewSQLDB(sqlplugin.DbKindMain, cfg, resolver.NewNoopResolver(), logger, metrics.NoopMetricsHandler)
	require.NoError(t, err)
	t.Cleanup(func() { _ = taskStore.Close() })

	serializer := serialization.NewSerializer()
	return &testEnv{
		taskStore:    taskStore,
		shardManager: p.NewShardManager(shardStore, serializer),
		executionManager: p.NewExecutionManager(
			executionStore,
			serializer,
			nil,
			logger,
			dynamicconfig.GetIntPropertyFn(4*1024*1024),
			dynamicconfig.GetBoolPropertyFn(false),
		),
		registry: tasks.NewDefaultTaskCategoryRegistry(),
	}
}

func (e *testEnv) resharder(oldCount, newCount int32) *reshard.Resharder {
	return &reshard.Resharder{
		ExecutionManager: e.executionManager,
		ShardManager:     e.shardManager,
		TaskStore:        e.taskStore,
		Registry:         e.registry,
		OldShardCount:    oldCount,
		NewShardCount:    newCount,
		Logger:           log.NewNoopLogger(),
	}
}

// seedWorkflows writes one transfer task per workflow onto whichever shard owns it under
// shardCount, mimicking what the running server would have persisted before the count changed.
func (e *testEnv) seedWorkflows(ctx context.Context, t *testing.T, count int, shardCount int32) []tasks.Task {
	t.Helper()

	var seeded []tasks.Task
	taskID := int64(1)
	for range count {
		task := &tasks.CloseExecutionTask{
			WorkflowKey:         definition.NewWorkflowKey(uuid.NewString(), uuid.NewString(), uuid.NewString()),
			TaskID:              taskID,
			VisibilityTimestamp: time.Now().UTC(),
		}
		taskID++

		e.writeTask(ctx, t, int32(tasks.GetShardIDForTask(task, int(shardCount))), tasks.CategoryTransfer, task)
		seeded = append(seeded, task)
	}
	return seeded
}

// seedTimers is seedWorkflows for a scheduled category, whose tasks are ordered by fire time rather
// than by task ID and so page through the store differently.
func (e *testEnv) seedTimers(ctx context.Context, t *testing.T, count int, shardCount int32) []tasks.Task {
	t.Helper()

	base := time.Now().UTC().Truncate(time.Millisecond)
	var seeded []tasks.Task
	for i := range count {
		seeded = append(seeded, e.seedTimerAt(ctx, t, int64(i+1), base.Add(time.Duration(i)*time.Second), shardCount))
	}
	return seeded
}

func (e *testEnv) seedTimerAt(ctx context.Context, t *testing.T, taskID int64, fireTime time.Time, shardCount int32) tasks.Task {
	t.Helper()

	task := &tasks.UserTimerTask{
		WorkflowKey:         definition.NewWorkflowKey(uuid.NewString(), uuid.NewString(), uuid.NewString()),
		TaskID:              taskID,
		VisibilityTimestamp: fireTime,
	}
	e.writeTask(ctx, t, int32(tasks.GetShardIDForTask(task, int(shardCount))), tasks.CategoryTimer, task)
	return task
}

func (e *testEnv) writeTask(ctx context.Context, t *testing.T, shardID int32, category tasks.Category, task tasks.Task) {
	t.Helper()

	resp, err := e.shardManager.GetOrCreateShard(ctx, &p.GetOrCreateShardRequest{
		ShardID:          shardID,
		LifecycleContext: ctx,
	})
	require.NoError(t, err)

	require.NoError(t, e.executionManager.AddHistoryTasks(ctx, &p.AddHistoryTasksRequest{
		ShardID:     shardID,
		RangeID:     resp.ShardInfo.RangeId,
		NamespaceID: task.GetNamespaceID(),
		WorkflowID:  task.GetWorkflowID(),
		Tasks:       map[tasks.Category][]tasks.Task{category: {task}},
	}))
}

// tasksOnShard returns every task of a category currently stored under shardID.
func (e *testEnv) tasksOnShard(ctx context.Context, t *testing.T, category tasks.Category, shardID int32) []tasks.Task {
	t.Helper()

	minKey, maxKey := tasks.NewImmediateKey(0), tasks.NewImmediateKey(tasks.MaximumKey.TaskID)
	if category.Type() == tasks.CategoryTypeScheduled {
		minKey, maxKey = tasks.NewKey(tasks.DefaultFireTime, 0), tasks.NewKey(tasks.MaximumKey.FireTime, 0)
	}

	var all []tasks.Task
	var pageToken []byte
	for {
		resp, err := e.executionManager.GetHistoryTasks(ctx, &p.GetHistoryTasksRequest{
			ShardID:             shardID,
			TaskCategory:        category,
			InclusiveMinTaskKey: minKey,
			ExclusiveMaxTaskKey: maxKey,
			BatchSize:           100,
			NextPageToken:       pageToken,
		})
		require.NoError(t, err)
		all = append(all, resp.Tasks...)
		if len(resp.NextPageToken) == 0 {
			return all
		}
		pageToken = resp.NextPageToken
	}
}

// assertEveryTaskOnItsOwningShard is the property that matters: after resharding, every task must be
// readable from the shard that the new count assigns to its workflow, and from nowhere else.
func (e *testEnv) assertEveryTaskOnItsOwningShard(ctx context.Context, t *testing.T, category tasks.Category, expected []tasks.Task, shardCount int32) {
	t.Helper()

	found := make(map[string]int32)
	total := 0
	for shardID := int32(1); shardID <= shardCount; shardID++ {
		for _, task := range e.tasksOnShard(ctx, t, category, shardID) {
			total++
			key := task.GetNamespaceID() + "/" + task.GetWorkflowID()
			_, duplicate := found[key]
			require.False(t, duplicate, "task for %v exists on more than one shard", key)
			found[key] = shardID
		}
	}
	require.Equal(t, len(expected), total, "task count changed during resharding")

	for _, task := range expected {
		key := task.GetNamespaceID() + "/" + task.GetWorkflowID()
		shardID, ok := found[key]
		require.True(t, ok, "task for %v was lost", key)
		require.Equal(t, int32(tasks.GetShardIDForTask(task, int(shardCount))), shardID,
			"task for %v is not on the shard that now owns it", key)
	}
}

func TestReshard_MovesTasksToTheirNewOwner(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	seeded := env.seedWorkflows(ctx, t, 200, oldShardCount)
	// The setup is only meaningful if some of these tasks are about to become misplaced.
	misplaced := 0
	for _, task := range seeded {
		if tasks.GetShardIDForTask(task, oldShardCount) != tasks.GetShardIDForTask(task, newShardCount) {
			misplaced++
		}
	}
	require.NotZero(t, misplaced, "no task would move; the fixture proves nothing")

	stats, err := env.resharder(oldShardCount, newShardCount).Run(ctx)
	require.NoError(t, err)
	require.Equal(t, misplaced, stats.TasksMoved)
	require.Equal(t, int(newShardCount-oldShardCount), stats.ShardsPrepared)

	env.assertEveryTaskOnItsOwningShard(ctx, t, tasks.CategoryTransfer, seeded, newShardCount)
}

func TestReshard_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	seeded := env.seedWorkflows(ctx, t, 100, oldShardCount)

	first, err := env.resharder(oldShardCount, newShardCount).Run(ctx)
	require.NoError(t, err)
	require.NotZero(t, first.TasksMoved)

	// A second run has nothing left to do, and must not disturb what the first one placed.
	second, err := env.resharder(oldShardCount, newShardCount).Run(ctx)
	require.NoError(t, err)
	require.Zero(t, second.TasksMoved)

	env.assertEveryTaskOnItsOwningShard(ctx, t, tasks.CategoryTransfer, seeded, newShardCount)
}

func TestReshard_NewShardRangeIDExceedsMigratedTaskIDs(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	// Advance the source shards' range IDs so the seeded tasks carry high IDs, the situation in
	// which a new shard starting from range 0 would hand out IDs below what it inherited.
	for shardID := int32(1); shardID <= oldShardCount; shardID++ {
		resp, err := env.shardManager.GetOrCreateShard(ctx, &p.GetOrCreateShardRequest{
			ShardID:          shardID,
			LifecycleContext: ctx,
		})
		require.NoError(t, err)
		info := resp.ShardInfo
		previous := info.RangeId
		info.RangeId += 10
		require.NoError(t, env.shardManager.UpdateShard(ctx, &p.UpdateShardRequest{
			ShardInfo:       info,
			PreviousRangeID: previous,
		}))
	}

	env.seedWorkflows(ctx, t, 100, oldShardCount)
	_, err := env.resharder(oldShardCount, newShardCount).Run(ctx)
	require.NoError(t, err)

	for shardID := oldShardCount + 1; shardID <= newShardCount; shardID++ {
		source := (shardID-1)%oldShardCount + 1
		sourceResp, err := env.shardManager.GetOrCreateShard(ctx, &p.GetOrCreateShardRequest{
			ShardID:          int32(source),
			LifecycleContext: ctx,
		})
		require.NoError(t, err)
		resp, err := env.shardManager.GetOrCreateShard(ctx, &p.GetOrCreateShardRequest{
			ShardID:          int32(shardID),
			LifecycleContext: ctx,
		})
		require.NoError(t, err)
		require.Greater(t, resp.ShardInfo.RangeId, sourceResp.ShardInfo.RangeId,
			"shard %v must start beyond the range its source is issuing from", shardID)

		// The property the range is there to carry: the first ID the new shard hands out is above
		// every ID it inherited. rangeSizeBits is the shift the server's key generator applies.
		const rangeSizeBits = 20
		firstIssuedTaskID := resp.ShardInfo.RangeId << rangeSizeBits
		for _, task := range env.tasksOnShard(ctx, t, tasks.CategoryTransfer, int32(shardID)) {
			require.Less(t, task.GetTaskID(), firstIssuedTaskID,
				"shard %v would reissue task ID %v", shardID, task.GetTaskID())
		}
	}
}

func TestReshard_MovesScheduledTasks(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	seeded := env.seedTimers(ctx, t, 200, oldShardCount)
	misplaced := 0
	for _, task := range seeded {
		if tasks.GetShardIDForTask(task, oldShardCount) != tasks.GetShardIDForTask(task, newShardCount) {
			misplaced++
		}
	}
	require.NotZero(t, misplaced, "no task would move; the fixture proves nothing")

	stats, err := env.resharder(oldShardCount, newShardCount).Run(ctx)
	require.NoError(t, err)
	require.Equal(t, misplaced, stats.TasksMoved)

	env.assertEveryTaskOnItsOwningShard(ctx, t, tasks.CategoryTimer, seeded, newShardCount)
}

// TestReshard_MovesScheduledTasksSharingAFireTime covers what a fire-time lower bound cannot page
// through: more tasks at one instant than a single batch holds.
func TestReshard_MovesScheduledTasksSharingAFireTime(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	fireTime := time.Now().UTC().Truncate(time.Millisecond)
	var seeded []tasks.Task
	for i := range 50 {
		seeded = append(seeded, env.seedTimerAt(ctx, t, int64(i+1), fireTime, oldShardCount))
	}

	resharder := env.resharder(oldShardCount, newShardCount)
	resharder.BatchSize = 10
	_, err := resharder.Run(ctx)
	require.NoError(t, err)

	env.assertEveryTaskOnItsOwningShard(ctx, t, tasks.CategoryTimer, seeded, newShardCount)
}

// abortingTaskStore fails the move after a few have gone through, standing in for a run that is
// killed partway.
type abortingTaskStore struct {
	reshard.TaskStore
	mu    sync.Mutex
	after int
}

func (m *abortingTaskStore) aborted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.after--
	return m.after < 0
}

func (m *abortingTaskStore) UpdateShardIDOfTransferTask(ctx context.Context, filter sqlplugin.TransferTasksFilter, newShardID int32) (dbsql.Result, error) {
	if m.aborted() {
		return nil, errAborted
	}
	return m.TaskStore.UpdateShardIDOfTransferTask(ctx, filter, newShardID)
}

func (m *abortingTaskStore) UpdateShardIDOfTimerTask(ctx context.Context, filter sqlplugin.TimerTasksFilter, newShardID int32) (dbsql.Result, error) {
	if m.aborted() {
		return nil, errAborted
	}
	return m.TaskStore.UpdateShardIDOfTimerTask(ctx, filter, newShardID)
}

var errAborted = errors.New("aborted")

// assertResumesAnInterruptedRun kills a run partway and re-runs it, which must land on the same
// state as an uninterrupted one. Since a move is a single statement, being interrupted can leave a
// task under either shard but never under both and never under neither.
func assertResumesAnInterruptedRun(t *testing.T, category tasks.Category, seed func(*testEnv, context.Context) []tasks.Task) {
	t.Helper()

	ctx := context.Background()
	env := newTestEnv(t)
	seeded := seed(env, ctx)

	interrupted := env.resharder(oldShardCount, newShardCount)
	interrupted.TaskStore = &abortingTaskStore{TaskStore: env.taskStore, after: 3}
	interrupted.Concurrency = 1
	stats, err := interrupted.Run(ctx)
	require.ErrorIs(t, err, errAborted)
	require.Equal(t, 3, stats.TasksMoved, "the fixture must leave the run half done")

	_, err = env.resharder(oldShardCount, newShardCount).Run(ctx)
	require.NoError(t, err)

	env.assertEveryTaskOnItsOwningShard(ctx, t, category, seeded, newShardCount)
}

func TestReshard_ResumesAnInterruptedRun(t *testing.T) {
	assertResumesAnInterruptedRun(t, tasks.CategoryTransfer, func(env *testEnv, ctx context.Context) []tasks.Task {
		return env.seedWorkflows(ctx, t, 100, oldShardCount)
	})
}

func TestReshard_ResumesAnInterruptedScheduledRun(t *testing.T) {
	assertResumesAnInterruptedRun(t, tasks.CategoryTimer, func(env *testEnv, ctx context.Context) []tasks.Task {
		return env.seedTimers(ctx, t, 100, oldShardCount)
	})
}

// TestReshard_MovesArchivalTasks pins the category set to what can be on disk rather than to what
// the running config enables. Archival tasks outlive the setting that produced them: the queue that
// drains them is registered only while archival is statically enabled, so disabling it leaves them
// in place with nothing consuming them.
func TestReshard_MovesArchivalTasks(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	env.registry = reshard.PersistedCategories()

	base := time.Now().UTC().Truncate(time.Millisecond)
	var seeded []tasks.Task
	for i := range 100 {
		task := &tasks.ArchiveExecutionTask{
			WorkflowKey:         definition.NewWorkflowKey(uuid.NewString(), uuid.NewString(), uuid.NewString()),
			TaskID:              int64(i + 1),
			VisibilityTimestamp: base.Add(time.Duration(i) * time.Second),
		}
		env.writeTask(ctx, t, int32(tasks.GetShardIDForTask(task, oldShardCount)), tasks.CategoryArchival, task)
		seeded = append(seeded, task)
	}

	stats, err := env.resharder(oldShardCount, newShardCount).Run(ctx)
	require.NoError(t, err)
	require.NotZero(t, stats.TasksMoved)

	env.assertEveryTaskOnItsOwningShard(ctx, t, tasks.CategoryArchival, seeded, newShardCount)
}

// TestReshard_DryRunReportsWithoutWriting checks the dry run against the real thing: it must report
// the same move count while leaving every task where the old count put it.
func TestReshard_DryRunReportsWithoutWriting(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	seeded := env.seedWorkflows(ctx, t, 200, oldShardCount)

	resharder := env.resharder(oldShardCount, newShardCount)
	resharder.DryRun = true
	dry, err := resharder.Run(ctx)
	require.NoError(t, err)
	require.NotZero(t, dry.TasksMoved, "the fixture proves nothing if nothing would move")
	require.Zero(t, dry.ShardsPrepared)
	env.assertEveryTaskOnItsOwningShard(ctx, t, tasks.CategoryTransfer, seeded, oldShardCount)

	applied, err := env.resharder(oldShardCount, newShardCount).Run(ctx)
	require.NoError(t, err)
	require.Equal(t, dry.TasksMoved, applied.TasksMoved, "the dry run must predict what the real one does")
	env.assertEveryTaskOnItsOwningShard(ctx, t, tasks.CategoryTransfer, seeded, newShardCount)
}

// TestReshard_OutcomeDoesNotDependOnConcurrency pins the claim that lets shards run in parallel:
// their destination sets are disjoint, so widening the run changes only how long it takes.
func TestReshard_OutcomeDoesNotDependOnConcurrency(t *testing.T) {
	for _, concurrency := range []int{1, 16} {
		t.Run(strconv.Itoa(concurrency), func(t *testing.T) {
			ctx := context.Background()
			env := newTestEnv(t)
			seeded := env.seedWorkflows(ctx, t, 200, oldShardCount)

			resharder := env.resharder(oldShardCount, newShardCount)
			resharder.Concurrency = concurrency
			stats, err := resharder.Run(ctx)
			require.NoError(t, err)
			require.Equal(t, len(seeded), stats.TasksScanned)

			env.assertEveryTaskOnItsOwningShard(ctx, t, tasks.CategoryTransfer, seeded, newShardCount)
		})
	}
}

// takeoverExecutionManager stands in for a history host that reclaims a shard while the tool is
// scanning it, which is what running the tool against a live cluster looks like.
type takeoverExecutionManager struct {
	p.ExecutionManager
	once     sync.Once
	takeOver func() error
	err      error
}

func (m *takeoverExecutionManager) GetHistoryTasks(
	ctx context.Context,
	request *p.GetHistoryTasksRequest,
) (*p.GetHistoryTasksResponse, error) {
	m.once.Do(func() { m.err = m.takeOver() })
	if m.err != nil {
		return nil, m.err
	}
	return m.ExecutionManager.GetHistoryTasks(ctx, request)
}

func TestReshard_ReportsAShardTakenOverDuringTheRun(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	env.seedWorkflows(ctx, t, 100, oldShardCount)

	resharder := env.resharder(oldShardCount, newShardCount)
	resharder.ExecutionManager = &takeoverExecutionManager{
		ExecutionManager: env.executionManager,
		takeOver: func() error {
			resp, err := env.shardManager.GetOrCreateShard(ctx, &p.GetOrCreateShardRequest{
				ShardID:          1,
				LifecycleContext: ctx,
			})
			if err != nil {
				return err
			}
			previousRangeID := resp.ShardInfo.RangeId
			resp.ShardInfo.RangeId = previousRangeID + 1
			return env.shardManager.UpdateShard(ctx, &p.UpdateShardRequest{
				ShardInfo:       resp.ShardInfo,
				PreviousRangeID: previousRangeID,
			})
		},
	}

	_, err := resharder.Run(ctx)
	require.ErrorContains(t, err, "shard 1 was taken over during the run")
}

func TestReshard_RejectsIllegalTransitions(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	_, err := env.resharder(8, 4).Run(ctx)
	require.Error(t, err, "shrinking must be refused")

	_, err = env.resharder(4, 6).Run(ctx)
	require.Error(t, err, "a non-multiple target must be refused")

	stats, err := env.resharder(4, 4).Run(ctx)
	require.NoError(t, err, "an unchanged count is a no-op, not an error")
	require.Zero(t, stats.TasksMoved)
}
