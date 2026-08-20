package iface

import "github.com/superdurable/temporal-server/common/metrics"

var (
	WorkerControllerInstanceCreated                    = metrics.NewCounterDef("worker_controller_instance_created")
	WorkerControllerInstanceVisibilityQueryCount       = metrics.NewCounterDef("worker_controller_instance_visibility_query_count")
	WorkerControllerInstanceProcessTaskMatchErrorCount = metrics.NewCounterDef("worker_controller_instance_task_match_error_count")
)
