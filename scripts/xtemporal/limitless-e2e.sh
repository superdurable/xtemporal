#!/usr/bin/env bash
# Provisions a temporary Aurora PostgreSQL Limitless cluster, runs the opt-in Limitless test layers
# against it, and always deletes every AWS resource it created, even when a step fails.
#
# The shard group is public and its security group only admits the caller's current public IP,
# because the tests run from the caller's machine. Use it for validation only, never for production.
# Visibility cannot live in the Limitless database (GENERATED STORED columns are unsupported on its
# standard tables), so a throwaway local PostgreSQL 16 container serves as the visibility store.
#
# Usage: scripts/xtemporal/limitless-e2e.sh
# Optional env:
#   AWS_PROFILE (default superdurable-write), AWS_REGION (default us-west-2)
#   LIMITLESS_E2E_ENGINE_VERSION (default: latest 16.x-limitless in the region)
#   LIMITLESS_E2E_MAX_ACU (default 64)
#   LIMITLESS_E2E_STEPS (default "sqltool dsn persistence functional")
#   LIMITLESS_E2E_FUNCTIONAL_RUN (go test -run regex for the functional layer)
#   LIMITLESS_E2E_FUNCTIONAL_SKIP (go test -skip regex; set it empty when running inside the cluster's VPC)
#   LIMITLESS_E2E_KEEP=1 skips teardown for debugging; delete the resources yourself afterwards.
set -euo pipefail

export AWS_PROFILE=${AWS_PROFILE:-superdurable-write}
export AWS_REGION=${AWS_REGION:-us-west-2}
export AWS_PAGER=""

run_id=xt-limitless-e2e-$(date +%Y%m%d%H%M%S)
cluster_id=$run_id
shard_group_id=$run_id-sg
role_name=$run_id-monitoring
sg_name=$run_id
# shellcheck disable=SC2054 # AWS CLI tag syntax uses commas inside each element.
tags=(Key=project,Value=xtemporal-limitless-e2e Key=run,Value="$run_id")
log_dir=${LIMITLESS_E2E_LOG_DIR:-/tmp/$run_id}
max_acu=${LIMITLESS_E2E_MAX_ACU:-64}
steps=${LIMITLESS_E2E_STEPS:-"sqltool dsn persistence functional"}
functional_run=${LIMITLESS_E2E_FUNCTIONAL_RUN:-'^(TestWorkflowTestSuite|TestSignalWorkflowTestSuiteLegacy|TestMaxBufferedEventSuite|TestActivityTestSuite|TestChildWorkflowSuite|TestContinueAsNewTestSuite|TestAcquireShardSuite)$'}
# These tests have tight wall-clock budgets (e.g. 101 serial signals within 20s). They fail the same way on
# vanilla PostgreSQL with postgres12_pgx behind a 30ms-RTT proxy and pass with postgres12_limitless at LAN
# latency, so they are skipped by default when the cluster is reached across the internet.
functional_skip=${LIMITLESS_E2E_FUNCTIONAL_SKIP-'^(TestMaxBufferedEventSuite|TestChildWorkflowSuite|TestContinueAsNewTestSuite)$/^(TestMaxBufferedEventsLimit|TestCronChildWorkflowExecution|TestChildWorkflowWithContinueAsNewParentTerminate)$'}
repo_root=$(git rev-parse --show-toplevel)
mkdir -p "$log_dir"
cd "$repo_root"

log() { printf '%s %s\n' "$(date +%T)" "$*" | tee -a "$log_dir/e2e.log" >&2; }

created_role=false
sg_id=""
visibility_container=$run_id-visibility
visibility_port=${LIMITLESS_E2E_VISIBILITY_PORT:-55439}
rdsosmetrics_existed=$(aws logs describe-log-groups --log-group-name-prefix RDSOSMetrics --query 'length(logGroups)' --output text)

cleanup() {
  local status=$?
  set +e
  if [[ ${LIMITLESS_E2E_KEEP:-} == 1 ]]; then
    log "LIMITLESS_E2E_KEEP=1: leaving run $run_id in place"
    exit "$status"
  fi
  log "teardown of $run_id"
  docker rm -f "$visibility_container" >/dev/null 2>&1
  if aws rds describe-db-shard-groups --db-shard-group-identifier "$shard_group_id" >/dev/null 2>&1; then
    aws rds delete-db-shard-group --db-shard-group-identifier "$shard_group_id" >/dev/null
    while aws rds describe-db-shard-groups --db-shard-group-identifier "$shard_group_id" >/dev/null 2>&1; do sleep 20; done
  fi
  if aws rds describe-db-clusters --db-cluster-identifier "$cluster_id" >/dev/null 2>&1; then
    # The cluster stays "modifying" for a while after its shard group is deleted; the AWS waiter treats
    # that as terminal, so retry the delete and poll until the cluster is gone.
    for _ in $(seq 1 60); do
      aws rds delete-db-cluster --db-cluster-identifier "$cluster_id" --skip-final-snapshot --delete-automated-backups >/dev/null 2>&1 && break
      sleep 20
    done
    for _ in $(seq 1 120); do
      aws rds describe-db-clusters --db-cluster-identifier "$cluster_id" >/dev/null 2>&1 || break
      sleep 20
    done
  fi
  if [[ -n $sg_id ]]; then
    # ENIs of a just-deleted cluster can hold the security group for a short while.
    for _ in $(seq 1 30); do aws ec2 delete-security-group --group-id "$sg_id" 2>/dev/null && break; sleep 20; done
  fi
  if [[ $created_role == true ]]; then
    aws iam detach-role-policy --role-name "$role_name" --policy-arn arn:aws:iam::aws:policy/service-role/AmazonRDSEnhancedMonitoringRole
    aws iam delete-role --role-name "$role_name"
  fi
  aws logs describe-log-groups --log-group-name-prefix "/aws/rds/cluster/$cluster_id" --query 'logGroups[].logGroupName' --output text |
    tr '\t' '\n' | while read -r group; do [[ -n $group ]] && aws logs delete-log-group --log-group-name "$group"; done
  # shellcheck disable=SC2016 # JMESPath literal, not a shell expansion.
  if [[ $rdsosmetrics_existed == 0 ]] &&
    [[ $(aws rds describe-db-instances --query 'length(DBInstances[?MonitoringInterval>`0`])' --output text) == 0 ]]; then
    aws logs delete-log-group --log-group-name RDSOSMetrics 2>/dev/null
  fi
  local leftovers=()
  aws rds describe-db-shard-groups --db-shard-group-identifier "$shard_group_id" >/dev/null 2>&1 && leftovers+=("shard group $shard_group_id")
  aws rds describe-db-clusters --db-cluster-identifier "$cluster_id" >/dev/null 2>&1 && leftovers+=("cluster $cluster_id")
  [[ $(aws rds describe-db-cluster-automated-backups --query "length(DBClusterAutomatedBackups[?DBClusterIdentifier=='$cluster_id'])" --output text) != 0 ]] && leftovers+=("automated backups of $cluster_id")
  [[ -n $sg_id ]] && aws ec2 describe-security-groups --group-ids "$sg_id" >/dev/null 2>&1 && leftovers+=("security group $sg_id")
  [[ $created_role == true ]] && aws iam get-role --role-name "$role_name" >/dev/null 2>&1 && leftovers+=("IAM role $role_name")
  if ((${#leftovers[@]})); then
    log "ERROR: resources left behind, delete them manually: ${leftovers[*]}"
    status=1
  else
    log "teardown complete; no resources left"
  fi
  exit "$status"
}
trap cleanup EXIT

engine_version=${LIMITLESS_E2E_ENGINE_VERSION:-$(aws rds describe-db-engine-versions --engine aurora-postgresql \
  --query "DBEngineVersions[?contains(EngineVersion,'limitless')].EngineVersion" --output text | tr '\t' '\n' | sort -V | tail -1)}
account_id=$(aws sts get-caller-identity --query Account --output text)
my_ip=$(curl -fsS https://checkip.amazonaws.com)
vpc_id=$(aws ec2 describe-vpcs --filters Name=isDefault,Values=true --query 'Vpcs[0].VpcId' --output text)
password=$(openssl rand -hex 20)
log "run $run_id: engine $engine_version, max ACU $max_acu, region $AWS_REGION, logs in $log_dir"

aws iam create-role --role-name "$role_name" --tags "${tags[@]}" \
  --assume-role-policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"monitoring.rds.amazonaws.com"},"Action":"sts:AssumeRole"}]}' >/dev/null
created_role=true
aws iam attach-role-policy --role-name "$role_name" --policy-arn arn:aws:iam::aws:policy/service-role/AmazonRDSEnhancedMonitoringRole

sg_id=$(aws ec2 create-security-group --group-name "$sg_name" --description "xtemporal limitless e2e $run_id" --vpc-id "$vpc_id" \
  --tag-specifications "ResourceType=security-group,Tags=[{Key=project,Value=xtemporal-limitless-e2e},{Key=run,Value=$run_id}]" \
  --query GroupId --output text)
aws ec2 authorize-security-group-ingress --group-id "$sg_id" --protocol tcp --port 5432 --cidr "$my_ip/32" >/dev/null

# A new monitoring role takes a few seconds to become assumable by RDS.
for attempt in $(seq 1 10); do
  if aws rds create-db-cluster --db-cluster-identifier "$cluster_id" \
    --engine aurora-postgresql --engine-version "$engine_version" \
    --storage-type aurora-iopt1 --cluster-scalability-type limitless \
    --master-username tadmin --master-user-password "$password" \
    --vpc-security-group-ids "$sg_id" \
    --enable-performance-insights --performance-insights-retention-period 31 \
    --monitoring-interval 5 --monitoring-role-arn "arn:aws:iam::$account_id:role/$role_name" \
    --enable-cloudwatch-logs-exports postgresql --tags "${tags[@]}" >/dev/null 2>"$log_dir/create-cluster.err"; then
    break
  fi
  [[ $attempt == 10 ]] && { cat "$log_dir/create-cluster.err" >&2; exit 1; }
  sleep 15
done
aws rds wait db-cluster-available --db-cluster-identifier "$cluster_id"
aws rds create-db-shard-group --db-shard-group-identifier "$shard_group_id" --db-cluster-identifier "$cluster_id" \
  --max-acu "$max_acu" --min-acu 16 --compute-redundancy 0 --publicly-accessible --tags "${tags[@]}" >/dev/null
log "shard group creating; billing starts now"
until [[ $(aws rds describe-db-shard-groups --db-shard-group-identifier "$shard_group_id" --query 'DBShardGroups[0].Status' --output text) == available ]]; do sleep 30; done
endpoint=$(aws rds describe-db-shard-groups --db-shard-group-identifier "$shard_group_id" --query 'DBShardGroups[0].Endpoint' --output text)
log "shard group available at $endpoint"

docker run -d --name "$visibility_container" -p "127.0.0.1:$visibility_port:5432" \
  -e POSTGRES_USER=temporal -e POSTGRES_PASSWORD=temporal postgres:16 >/dev/null
until docker exec "$visibility_container" pg_isready -U temporal >/dev/null 2>&1; do sleep 2; done
export LIMITLESS_VISIBILITY_HOST=127.0.0.1 LIMITLESS_VISIBILITY_PORT=$visibility_port \
  LIMITLESS_VISIBILITY_USER=temporal LIMITLESS_VISIBILITY_PASSWORD=temporal LIMITLESS_VISIBILITY_SSLMODE=disable

go build -o "$log_dir/temporal-sql-tool" ./cmd/tools/sql
sql_tool() {
  SQL_PASSWORD=$password "$log_dir/temporal-sql-tool" --endpoint "$endpoint" --port 5432 --user tadmin \
    --connect-attributes sslmode=require --plugin postgres12_limitless "$@"
}
sql_tool --database temporal create-database --defaultdb postgres_limitless >>"$log_dir/e2e.log" 2>&1
sql_tool --database temporal_scratch create-database --defaultdb postgres_limitless >>"$log_dir/e2e.log" 2>&1

# Persistence suites and functional clusters each create their own database through the admin database.
export LIMITLESS_HOST=$endpoint LIMITLESS_PORT=5432 LIMITLESS_USER=tadmin LIMITLESS_PASSWORD=$password \
  LIMITLESS_ADMIN_DATABASE=postgres_limitless LIMITLESS_SQL_TOOL_DATABASE=temporal
scratch_dsn="postgres://tadmin:$password@$endpoint:5432/temporal_scratch?sslmode=require"

failed=()
run_step() {
  local name=$1
  shift
  log "step $name: start"
  if "$@" >"$log_dir/$name.log" 2>&1; then
    log "step $name: PASS"
  else
    log "step $name: FAIL (see $log_dir/$name.log)"
    failed+=("$name")
  fi
}

for step in $steps; do
  case $step in
    sqltool) run_step sqltool scripts/xtemporal/limitless-sql-tool-verify.sh ;;
    dsn) run_step dsn env XTEMPORAL_LIMITLESS_TEST_DSN="$scratch_dsn" \
      go test -tags test_dep -count=1 -p 1 -v -run 'Postgres' ./x/limitless/ ./schema/limitless/ ;;
    # Limitless DDL takes cluster-wide locks and CREATE DATABASE is not transactional, so run serially.
    # TEMPORAL_DEBUG scales persistence-suite context timeouts: the tests run from the caller's machine
    # across the internet, and e.g. TestScanAllTrees writes 1002 trees serially under one 30s context.
    persistence) run_step persistence env LIMITLESS_TEST=aurora \
      go test -tags test_dep,TEMPORAL_DEBUG -count=1 -p 1 -parallel 1 -timeout 180m -v -run 'Limitless' ./common/persistence/tests/ ;;
    functional) run_step functional env LIMITLESS_TEST=aurora \
      go test -tags test_dep -count=1 -p 1 -parallel 1 -timeout 240m -v -run "$functional_run" -skip "$functional_skip" ./tests/ \
      -persistenceType=sql -persistenceDriver=postgres12_limitless ;;
    *) log "unknown step $step"; failed+=("$step") ;;
  esac
done

if ((${#failed[@]})); then
  log "FAILED steps: ${failed[*]}"
  exit 1
fi
log "all steps passed"
