// Package reshard moves pending history tasks to the shard that owns their workflow after the
// history shard count has been increased.
//
// Every table holding per-workflow state is keyed by the workflow itself and so is unaffected by a
// change in shard count. History tasks are the exception: they are stored under the shard that wrote
// them, and that shard is derived from the workflow's hash modulo the shard count. Raise the count
// and a task can end up under a shard whose owner no longer owns the workflow — the owner would poll
// a task it cannot execute, while the shard that should run it never sees it.
//
// This is an offline operation. It must run with the cluster stopped, after the configured shard
// count has been raised and before the servers are started again. It claims the shards it reads so
// that running it against a live cluster fails loudly rather than migrating half the tasks.
//
// It supports SQL stores only. Moving a task has to be one statement, and only the SQL layer can
// express one; see TaskStore.
package reshard

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/common/log"
	"github.com/superdurable/temporal-server/common/log/tag"
	"github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin"
	"github.com/superdurable/temporal-server/service/history/tasks"
	"golang.org/x/sync/errgroup"
)

const (
	defaultBatchSize = 1000

	// defaultConcurrency is how many old shards are relocated at once. It is meant to keep a normally
	// sized connection pool busy without being the thing that exhausts it.
	defaultConcurrency = 4
)

// PersistedCategories returns the default task categories plus archival, which together cover
// everything that can be sitting in the database.
//
// Archival is deliberately not taken from the server's registry, which carries it only while
// archival is statically enabled. Its queue processor is gated on that same registration, so turning
// archival off stops anything from draining archival tasks without removing the ones already
// written. Relocating a category no queue currently reads costs one empty query per shard; skipping
// one that turns out to be populated strands its tasks on shards that no longer own their workflows,
// and reports success while doing it.
func PersistedCategories() tasks.TaskCategoryRegistry {
	registry := tasks.NewDefaultTaskCategoryRegistry()
	registry.AddCategory(tasks.CategoryArchival)
	return registry
}

type (
	// TaskStore reassigns a task row to another shard, one method per task table.
	//
	// This is the SQL layer rather than the ExecutionManager the rest of the tool reads through,
	// because the move has to be a single UPDATE and nothing above sqlplugin can express one. That
	// makes the tool SQL-only: Cassandra keys task rows by shard, so there the same move would be a
	// write to one partition and a delete from another with no way to commit them together.
	TaskStore interface {
		UpdateShardIDOfHistoryImmediateTask(ctx context.Context, filter sqlplugin.HistoryImmediateTasksFilter, newShardID int32) (sql.Result, error)
		UpdateShardIDOfHistoryScheduledTask(ctx context.Context, filter sqlplugin.HistoryScheduledTasksFilter, newShardID int32) (sql.Result, error)
		UpdateShardIDOfTransferTask(ctx context.Context, filter sqlplugin.TransferTasksFilter, newShardID int32) (sql.Result, error)
		UpdateShardIDOfTimerTask(ctx context.Context, filter sqlplugin.TimerTasksFilter, newShardID int32) (sql.Result, error)
		UpdateShardIDOfReplicationTask(ctx context.Context, filter sqlplugin.ReplicationTasksFilter, newShardID int32) (sql.Result, error)
		UpdateShardIDOfVisibilityTask(ctx context.Context, filter sqlplugin.VisibilityTasksFilter, newShardID int32) (sql.Result, error)
	}

	// Resharder moves history tasks from the shard layout implied by OldShardCount to the one
	// implied by NewShardCount.
	Resharder struct {
		ExecutionManager persistence.ExecutionManager
		ShardManager     persistence.ShardManager
		TaskStore        TaskStore
		Registry         tasks.TaskCategoryRegistry
		OldShardCount    int32
		NewShardCount    int32
		BatchSize        int
		Concurrency      int
		DryRun           bool
		Logger           log.Logger

		fenced map[int32]int64
	}

	// Stats reports what a run did, for the operator to sanity check against expectations.
	Stats struct {
		ShardsPrepared int
		TasksScanned   int
		TasksMoved     int
	}
)

// Run creates the shard records introduced by the larger count and then relocates every history task
// that no longer belongs to the shard it is stored under.
//
// It is safe to re-run and safe to interrupt: a task already sitting on its correct shard is left
// alone, and moving one is a single statement that reassigns the row rather than a write to the
// destination paired with a delete from the source, so a task is never on two shards nor on none.
//
// With DryRun set it performs the same scan and reports the same counts while writing nothing.
func (r *Resharder) Run(ctx context.Context) (Stats, error) {
	var stats Stats

	if err := persistence.ValidateHistoryShardCountTransition(r.OldShardCount, r.NewShardCount); err != nil {
		return stats, err
	}
	if r.OldShardCount == r.NewShardCount {
		r.Logger.Info("History shard count unchanged, nothing to reshard.",
			tag.NewInt32("shard-count", r.OldShardCount))
		return stats, nil
	}

	if !r.DryRun {
		if err := r.fenceSourceShards(ctx); err != nil {
			return stats, err
		}
		prepared, err := r.createNewShards(ctx)
		if err != nil {
			return stats, err
		}
		stats.ShardsPrepared = prepared
	}

	scanned, moved, err := r.reshardAllShards(ctx)
	stats.TasksScanned = scanned
	stats.TasksMoved = moved
	if err != nil {
		return stats, err
	}

	if !r.DryRun {
		if err := r.verifyFence(ctx); err != nil {
			return stats, err
		}
	}

	r.Logger.Info("Resharding complete.",
		tag.NewInt("shards-prepared", stats.ShardsPrepared),
		tag.NewInt("tasks-scanned", stats.TasksScanned),
		tag.NewInt("tasks-moved", stats.TasksMoved))
	return stats, nil
}

// fenceSourceShards claims every shard that predates the increase by advancing its range.
//
// The cluster has to be stopped for a reshard, and nothing available here can enforce that. Claiming
// the ranges makes a violation loud rather than silent: a history host still holding one of these
// shards fails its next write with ShardOwnershipLost, and reacquiring the shard moves its range past
// what was written here, which verifyFence reports at the end of the run. The burnt range costs
// nothing beyond the block of task IDs it would have issued.
func (r *Resharder) fenceSourceShards(ctx context.Context) error {
	r.fenced = make(map[int32]int64, r.OldShardCount)
	for shardID := int32(1); shardID <= r.OldShardCount; shardID++ {
		resp, err := r.ShardManager.GetOrCreateShard(ctx, &persistence.GetOrCreateShardRequest{
			ShardID:          shardID,
			LifecycleContext: ctx,
		})
		if err != nil {
			return fmt.Errorf("reading shard %v: %w", shardID, err)
		}

		previousRangeID := resp.ShardInfo.RangeId
		updated := resp.ShardInfo
		updated.RangeId = previousRangeID + 1
		if err := r.ShardManager.UpdateShard(ctx, &persistence.UpdateShardRequest{
			ShardInfo:       updated,
			PreviousRangeID: previousRangeID,
		}); err != nil {
			return fmt.Errorf("claiming shard %v, which a running cluster would prevent: %w", shardID, err)
		}
		r.fenced[shardID] = updated.RangeId
	}
	return nil
}

// verifyFence reports whether any shard claimed at the start of the run was taken back during it.
func (r *Resharder) verifyFence(ctx context.Context) error {
	for shardID := int32(1); shardID <= r.OldShardCount; shardID++ {
		resp, err := r.ShardManager.GetOrCreateShard(ctx, &persistence.GetOrCreateShardRequest{
			ShardID:          shardID,
			LifecycleContext: ctx,
		})
		if err != nil {
			return fmt.Errorf("re-reading shard %v: %w", shardID, err)
		}
		if claimed := r.fenced[shardID]; resp.ShardInfo.RangeId != claimed {
			return fmt.Errorf(
				"shard %v was taken over during the run, its range moving from %v to %v: the cluster "+
					"must be stopped, and what has been moved so far may be incomplete",
				shardID, claimed, resp.ShardInfo.RangeId)
		}
	}
	return nil
}

// createNewShards materialises the shard records that the larger count introduces.
//
// Each new shard s is seeded from the shard it split off from, which is the one s would have hashed
// to under the old count. Two things are carried over:
//
//   - range_id, incremented by one. Task IDs are derived from the range ID, so starting the new
//     shard at a range its source has already passed guarantees that every ID it hands out is above
//     every ID it inherits. Starting from zero would hand out IDs below the migrated tasks, and the
//     queue would treat those tasks as already acknowledged.
//   - queue_states, verbatim. The new shard's tasks are a subset of its source's, so an ack level
//     that was accurate for the whole set is still accurate — and conservative — for the subset.
func (r *Resharder) createNewShards(ctx context.Context) (int, error) {
	prepared := 0
	for shardID := r.OldShardCount + 1; shardID <= r.NewShardCount; shardID++ {
		sourceShardID := (shardID-1)%r.OldShardCount + 1

		source, err := r.ShardManager.GetOrCreateShard(ctx, &persistence.GetOrCreateShardRequest{
			ShardID:          sourceShardID,
			LifecycleContext: ctx,
		})
		if err != nil {
			return prepared, fmt.Errorf("reading source shard %v for new shard %v: %w", sourceShardID, shardID, err)
		}

		resp, err := r.ShardManager.GetOrCreateShard(ctx, &persistence.GetOrCreateShardRequest{
			ShardID: shardID,
			InitialShardInfo: &persistencespb.ShardInfo{
				ShardId:     shardID,
				RangeId:     source.ShardInfo.RangeId + 1,
				QueueStates: source.ShardInfo.QueueStates,
			},
			LifecycleContext: ctx,
		})
		if err != nil {
			return prepared, fmt.Errorf("creating shard %v: %w", shardID, err)
		}

		// Either the call above created the shard at the range we asked for, or it already existed
		// at or beyond it. Leave the latter alone: a partially completed run may have advanced it, and
		// rewinding would reissue task IDs already handed out.
		if resp.ShardInfo.RangeId >= source.ShardInfo.RangeId+1 {
			prepared++
			continue
		}

		// Read before writing: updated aliases resp.ShardInfo, so taking the previous range from it
		// afterwards would yield the range being written and the conditional update would never match.
		previousRangeID := resp.ShardInfo.RangeId
		updated := resp.ShardInfo
		updated.RangeId = source.ShardInfo.RangeId + 1
		updated.QueueStates = source.ShardInfo.QueueStates
		if err := r.ShardManager.UpdateShard(ctx, &persistence.UpdateShardRequest{
			ShardInfo:       updated,
			PreviousRangeID: previousRangeID,
		}); err != nil {
			return prepared, fmt.Errorf("seeding shard %v from shard %v: %w", shardID, sourceShardID, err)
		}
		prepared++
	}
	return prepared, nil
}

// reshardAllShards relocates the tasks of every shard the old count covered.
//
// Old shards are worked on concurrently because they do not interact: the tasks of old shard s can
// only land on {s, s+old, s+2*old, ...}, and those sets are disjoint across sources, so two shards
// being relocated at once contend for nothing but the database. Since this runs as downtime, how
// wide it goes is what decides how long that downtime is.
func (r *Resharder) reshardAllShards(ctx context.Context) (scanned int, moved int, err error) {
	concurrency := r.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}

	var mu sync.Mutex

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(concurrency)
	for shardID := int32(1); shardID <= r.OldShardCount; shardID++ {
		group.Go(func() error {
			s, m, err := r.reshardOneShard(groupCtx, shardID)

			// Count what this shard did even when it failed, so that an aborted run reports how far
			// it got rather than discarding the work the other shards completed.
			mu.Lock()
			scanned += s
			moved += m
			mu.Unlock()

			if err != nil {
				return fmt.Errorf("resharding tasks of shard %v: %w", shardID, err)
			}
			return nil
		})
	}
	return scanned, moved, group.Wait()
}

// reshardOneShard relocates every task of one old shard that now belongs elsewhere.
func (r *Resharder) reshardOneShard(ctx context.Context, shardID int32) (scanned int, moved int, err error) {
	for _, category := range r.Registry.GetCategories() {
		// The memory timer category backs the speculative workflow task queue, which never reaches
		// the database. It is in every registry regardless, so it is skipped here rather than left
		// out of one.
		if category.ID() == tasks.CategoryIDMemoryTimer {
			continue
		}

		s, m, err := r.reshardCategory(ctx, shardID, category)
		scanned += s
		moved += m
		if err != nil {
			return scanned, moved, fmt.Errorf("category %v: %w", category.Name(), err)
		}
	}
	return scanned, moved, nil
}

func (r *Resharder) reshardCategory(
	ctx context.Context,
	shardID int32,
	category tasks.Category,
) (scanned int, moved int, err error) {
	batchSize := r.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	// The scan pages with the store's own token, which is a position rather than an offset: an
	// immediate token is the last task ID plus one, a scheduled one the last (fire time, task ID)
	// pair. Deleting the rows behind it therefore cannot invalidate it, and the position advances
	// whether or not any given move removed its source row.
	//
	// A hand-rolled lower bound cannot replace it. Scheduled categories reject a task ID in the
	// range at all (see validateTaskRange), so the only bound expressible for them is a fire time,
	// which neither advances past tasks sharing one nor can skip them without dropping tasks.
	minKey, maxKey := categoryKeyRange(category)
	var pageToken []byte
	for {
		resp, err := r.ExecutionManager.GetHistoryTasks(ctx, &persistence.GetHistoryTasksRequest{
			ShardID:             shardID,
			TaskCategory:        category,
			InclusiveMinTaskKey: minKey,
			ExclusiveMaxTaskKey: maxKey,
			BatchSize:           batchSize,
			NextPageToken:       pageToken,
		})
		if err != nil {
			return scanned, moved, err
		}

		for _, task := range resp.Tasks {
			scanned++
			target := int32(tasks.GetShardIDForTask(task, int(r.NewShardCount)))
			if target == shardID {
				continue
			}
			if err := r.moveTask(ctx, shardID, target, category, task); err != nil {
				return scanned, moved, err
			}
			moved++
		}

		if len(resp.NextPageToken) == 0 {
			return scanned, moved, nil
		}
		pageToken = resp.NextPageToken
	}
}

// moveTask reassigns one task row to the shard that now owns its workflow.
//
// A task row differs from the row it needs to become only in shard_id, so the move is one UPDATE.
// Writing it to the new shard and deleting it from the old would be two statements that no store can
// commit together across shards, leaving the task on both if the run stopped in between.
func (r *Resharder) moveTask(
	ctx context.Context,
	fromShardID int32,
	toShardID int32,
	category tasks.Category,
	task tasks.Task,
) error {
	if r.DryRun {
		return nil
	}

	if err := r.updateShardIDOfTask(ctx, fromShardID, toShardID, category, task.GetKey()); err != nil {
		return fmt.Errorf("moving task %v from shard %v to shard %v: %w",
			task.GetKey(), fromShardID, toShardID, err)
	}
	return nil
}

// updateShardIDOfTask picks the table the task lives in and reassigns its row.
//
// Categories predating the generic history_immediate_tasks and history_scheduled_tasks tables still
// have tables of their own, which is the only reason this is a switch rather than two statements.
func (r *Resharder) updateShardIDOfTask(
	ctx context.Context,
	fromShardID int32,
	toShardID int32,
	category tasks.Category,
	key tasks.Key,
) error {
	var err error
	switch category.ID() {
	case tasks.CategoryIDTransfer:
		_, err = r.TaskStore.UpdateShardIDOfTransferTask(ctx, sqlplugin.TransferTasksFilter{
			ShardID: fromShardID,
			TaskID:  key.TaskID,
		}, toShardID)
	case tasks.CategoryIDVisibility:
		_, err = r.TaskStore.UpdateShardIDOfVisibilityTask(ctx, sqlplugin.VisibilityTasksFilter{
			ShardID: fromShardID,
			TaskID:  key.TaskID,
		}, toShardID)
	case tasks.CategoryIDReplication:
		_, err = r.TaskStore.UpdateShardIDOfReplicationTask(ctx, sqlplugin.ReplicationTasksFilter{
			ShardID: fromShardID,
			TaskID:  key.TaskID,
		}, toShardID)
	case tasks.CategoryIDTimer:
		_, err = r.TaskStore.UpdateShardIDOfTimerTask(ctx, sqlplugin.TimerTasksFilter{
			ShardID:             fromShardID,
			VisibilityTimestamp: key.FireTime,
			TaskID:              key.TaskID,
		}, toShardID)
	default:
		if category.Type() == tasks.CategoryTypeScheduled {
			_, err = r.TaskStore.UpdateShardIDOfHistoryScheduledTask(ctx, sqlplugin.HistoryScheduledTasksFilter{
				ShardID:             fromShardID,
				CategoryID:          int32(category.ID()),
				VisibilityTimestamp: key.FireTime,
				TaskID:              key.TaskID,
			}, toShardID)
			break
		}
		_, err = r.TaskStore.UpdateShardIDOfHistoryImmediateTask(ctx, sqlplugin.HistoryImmediateTasksFilter{
			ShardID:    fromShardID,
			CategoryID: int32(category.ID()),
			TaskID:     key.TaskID,
		}, toShardID)
	}
	return err
}

// categoryKeyRange returns a range covering every task a category can hold. Immediate categories are
// ordered by task ID alone; scheduled ones by fire time.
func categoryKeyRange(category tasks.Category) (minKey tasks.Key, maxKey tasks.Key) {
	if category.Type() == tasks.CategoryTypeScheduled {
		return tasks.NewKey(tasks.DefaultFireTime, 0), tasks.NewKey(tasks.MaximumKey.FireTime, 0)
	}
	return tasks.NewImmediateKey(0), tasks.NewImmediateKey(tasks.MaximumKey.TaskID)
}
