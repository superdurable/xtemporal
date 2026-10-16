# Dex dynamic config profile

The [Dex profile](../../config/dynamicconfig/xtemporal-dex-profile.yaml) narrows the Temporal APIs and background features used by Dex clusters. Dex searches workflows through `ListWorkflowExecutions` and uses Schedules v1 for blob cleanup. It does not use Count, legacy filtered open/closed lists, batch operations, Worker Versioning, standalone activities, or standalone Nexus operations. The Dex visibility store does not support Count or arbitrary `ORDER BY` queries.

| Key | Value | Effect |
| --- | --- | --- |
| `xtemporal.frontend.disableCountWorkflowExecutions` | `true` | Return `Unimplemented` from `CountWorkflowExecutions` and `CountSchedules` instead of querying the Dex visibility store. |
| `frontend.disableListVisibilityByFilter` | `true` | Reject legacy filtered open/closed workflow lists. This upstream switch also rejects `ListSchedules`. |
| `frontend.visibilityMaxPageSize` | `100` | Cap visibility pages at 100; callers can continue with page tokens. |
| `system.visibilityDisableOrderByClause` | `true` | Reject explicit `ORDER BY` clauses that the Dex visibility store does not support. |
| `system.enableDeployments` | `false` | Disable legacy deployments, which Dex does not use. |
| `system.enableDeploymentVersions` | `false` | Disable deployment versions, which Dex does not use. |
| `frontend.workerVersioningDataAPIs` | `false` | Disable worker versioning data APIs, which Dex does not use. |
| `frontend.workerVersioningWorkflowAPIs` | `false` | Disable worker versioning workflow APIs, which Dex does not use. |
| `frontend.workerVersioningRuleAPIs` | `false` | Disable worker versioning rule APIs, which Dex does not use. |
| `frontend.MaxConcurrentBatchOperationPerNamespace` | `0` | Set the batch concurrency ceiling to zero. With the Count switch enabled, `StartBatchOperation` returns `Unimplemented` before reaching this check. |
| `worker.enableNamespaceBatcher` | `false` | Stop the per namespace batcher worker, which Dex does not use. |
| `activity.enableStandalone` | `false` | Disable standalone activity APIs without disabling CHASM. |
| `nexusoperation.enableStandalone` | `false` | Disable standalone Nexus operation APIs without disabling CHASM. |

The Count switch applies at the frontend RPC handlers. Worker code that calls the visibility manager directly, including namespace deletion, remains able to count. The batcher uses the frontend Count API and is blocked by this switch; the profile also disables the batcher. `ScanWorkflowExecutions` remains available. The profile does not change `history.enableChasm` or `history.enableCHASMSchedulerCreation`; new schedules continue to use Schedules v1 because CHASM scheduler creation defaults to `false`. `ListSchedules` is unavailable because the requested `frontend.disableListVisibilityByFilter` setting also gates it; schedule creation and other schedule operations remain enabled.

## Apply the profile

Set `dynamicConfigClient.filepath` in the server's static config to the deployed copy of `xtemporal-dex-profile.yaml`. For the Docker image, set `DYNAMIC_CONFIG_FILE_PATH` to that path and mount the file there. Deploy the same file to every service host. The file based client reloads it at its configured `pollInterval`; restart services if their static config path changes. This profile applies cluster wide unless individual values are given namespace constraints. Do not apply it to clusters that need the disabled APIs.
