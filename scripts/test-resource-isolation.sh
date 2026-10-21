#!/usr/bin/env bash
set -euo pipefail

# Manual acceptance path for hosts with a genuinely delegated writable cgroup v2.
# Run with: ASTER_CGROUP_ROOT=/absolute/delegated/root bash scripts/test-resource-isolation.sh
# The script fails if any required integration test is skipped, so a privileged
# validation report cannot accidentally count a capability skip as a successful run.

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "resource-isolation validation requires Linux" >&2
  exit 2
fi
if [[ -z "${ASTER_CGROUP_ROOT:-}" ]]; then
  echo "set ASTER_CGROUP_ROOT to an explicitly delegated, writable cgroup v2 directory" >&2
  exit 2
fi
if [[ ! -d "$ASTER_CGROUP_ROOT" ]]; then
  echo "ASTER_CGROUP_ROOT is not a directory: $ASTER_CGROUP_ROOT" >&2
  exit 2
fi
ASTER_CGROUP_ROOT="$(cd "$ASTER_CGROUP_ROOT" && pwd -P)"
export ASTER_CGROUP_ROOT

for control in cgroup.controllers cgroup.subtree_control; do
  if [[ ! -r "$ASTER_CGROUP_ROOT/$control" ]]; then
    echo "cannot read $ASTER_CGROUP_ROOT/$control" >&2
    exit 2
  fi
done
for controller in cpu memory; do
  if ! grep -qw "$controller" "$ASTER_CGROUP_ROOT/cgroup.controllers"; then
    echo "required controller '$controller' is not available at $ASTER_CGROUP_ROOT" >&2
    exit 2
  fi
  if ! grep -qw "$controller" "$ASTER_CGROUP_ROOT/cgroup.subtree_control"; then
    echo "required controller '$controller' is not enabled for child cgroups at $ASTER_CGROUP_ROOT" >&2
    exit 2
  fi
done
if ! command -v go >/dev/null 2>&1; then
  echo "Go is required to run the resource-isolation tests" >&2
  exit 2
fi

output_dir="${ASTER_RESOURCE_TEST_OUTPUT_DIR:-${TMPDIR:-/tmp}/aster-resource-validation}"
mkdir -p "$output_dir"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
log_file="$output_dir/delegated-cgroup-v2-$stamp.log"

{
  echo "Aster delegated cgroup v2 resource-isolation validation"
  echo "timestamp_utc=$stamp"
  echo "kernel=$(uname -r)"
  echo "architecture=$(uname -m)"
  echo "go_version=$(go version)"
  echo "cgroup_root=$ASTER_CGROUP_ROOT"
  echo "cpu_poll_interval=10ms"
  printf 'cgroup.controllers='
  cat "$ASTER_CGROUP_ROOT/cgroup.controllers"
  printf 'cgroup.subtree_control='
  cat "$ASTER_CGROUP_ROOT/cgroup.subtree_control"
  echo
} | tee "$log_file"

if ! go test -count=1 -v ./internal/resource 2>&1 | tee -a "$log_file"; then
  echo "resource-isolation test suite failed; inspect $log_file" >&2
  exit 1
fi

required_tests=(
  TestRunEnforcesCumulativeCPUTimeWhenDelegatedCgroupIsAvailable
  TestRunEnforcesMemoryLimitWhenDelegatedCgroupIsAvailable
  TestRunConfiguresCPUQuotaBeforeWorkloadAndCleansUp
  TestRunCPUQuotaThrottlesAndReportsKernelMetrics
  TestRunConfiguresMemoryControlsAndCleansUp
  TestRunKillsDescendantsAndCleansUpOnCPUTimeLimit
  TestRunKillsDescendantsAndCleansUpOnContextDeadline
)
for test_name in "${required_tests[@]}"; do
  if ! grep -Fq -- "--- PASS: $test_name" "$log_file"; then
    echo "required integration test did not pass (it may have skipped): $test_name" >&2
    echo "inspect raw output: $log_file" >&2
    exit 1
  fi
done

if ! grep -Eq 'cpu_time_measurement budget_usec=[0-9]+ observed_usec=[0-9]+ overshoot_usec=[0-9]+ sample=post-termination cpu_poll_interval=10ms' "$log_file"; then
  echo "post-termination CPU-time overshoot measurement is missing from the raw log" >&2
  echo "inspect raw output: $log_file" >&2
  exit 1
fi

echo "all delegated-cgroup integration tests passed with a post-termination CPU-time measurement" | tee -a "$log_file"
echo "raw validation output: $log_file"
