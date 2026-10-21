#!/usr/bin/env python3
"""Repeatable Linux CLI benchmark for Native resource isolation."""

from __future__ import annotations

import argparse
import csv
import json
import math
import os
import platform
import re
import shutil
import signal
import statistics
import subprocess
import sys
import tempfile
import threading
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
POLL_SECONDS = 0.002
CPU_POLL_INTERVAL = "10ms"
CPU_TIME_LIMIT = "50ms"
DEFAULT_MEMORY_LIMIT = "256MiB"
DEFAULT_OOM_MEMORY_LIMIT = "64MiB"
DEFAULT_CPU_RATE = "1.0"
RUN_FIELDS = [
    "kind", "workload", "profile", "repetition", "timestamp_utc",
    "exit_code", "outcome", "termination_classification", "cgroup_cleanup_status", "wall_ms", "sampled_tree_rss_bytes",
    "cgroup_memory_peak_bytes", "binary_bytes", "cpu_budget_usec",
    "cpu_observed_usec", "cpu_overshoot_usec", "cpu_poll_interval",
    "stdout_json", "stderr_json",
]
WORKLOADS = {
    "launch-heavy": {
        "source": "print(0);\n",
        "expected": "0",
    },
    "cpu-bound": {
        "source": (
            "let mut count: i64 = 0;\n"
            "while count < 250000 {\n"
            "    count = count + 1;\n"
            "}\n"
            "print(count);\n"
        ),
        "expected": "250000",
    },
    "memory-bounded": {
        "source": (
            "fn grow(value: string) -> string {\n"
            "    let mut text = value;\n"
            "    let mut count: i64 = 0;\n"
            "    while count < 24 {\n"
            "        text = text + text;\n"
            "        count = count + 1;\n"
            "    }\n"
            "    return text;\n"
            "}\n"
            "let value0 = grow(\"x\");\n"
            "let value1 = grow(\"x\");\n"
            "print(len(value0) + len(value1));\n"
        ),
        "expected": "33554432",
    },
    "memory-oom": {
        "source": (
            "fn grow(value: string) -> string {\n"
            "    let mut text = value;\n"
            "    let mut count: i64 = 0;\n"
            "    while count < 24 {\n"
            "        text = text + text;\n"
            "        count = count + 1;\n"
            "    }\n"
            "    return text;\n"
            "}\n"
            "let value0 = grow(\"x\");\n"
            "let value1 = grow(\"x\");\n"
            "let value2 = grow(\"x\");\n"
            "let value3 = grow(\"x\");\n"
            "let value4 = grow(\"x\");\n"
            "print(len(value0) + len(value1) + len(value2) + len(value3) + len(value4));\n"
        ),
        "expected": "83886080",
    },
    "cpu-time-termination": {
        "source": (
            "let mut count: i64 = 0;\n"
            "while true {\n"
            "    count = count + 1;\n"
            "}\n"
        ),
        "expected": "",
    },
}


class MeasurementError(RuntimeError):
    pass


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds")


def positive_int_env(name: str, default: int) -> int:
    value = os.environ.get(name, str(default))
    try:
        parsed = int(value)
    except ValueError as exc:
        raise MeasurementError(f"{name} must be a positive integer, got {value!r}") from exc
    if parsed < 1:
        raise MeasurementError(f"{name} must be a positive integer, got {value!r}")
    return parsed


def read_text(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8").strip()
    except OSError:
        return ""


def preflight() -> tuple[Path, str]:
    if not sys.platform.startswith("linux"):
        raise MeasurementError("this harness requires Linux and cgroup v2")
    for tool in ("go", "git", "python3"):
        if not shutil.which(tool):
            raise MeasurementError(f"required tool not found: {tool}")
    raw_root = os.environ.get("ASTER_CGROUP_ROOT", "")
    if not raw_root:
        raise MeasurementError(
            "set ASTER_CGROUP_ROOT to an explicitly delegated writable cgroup v2 directory"
        )
    root = Path(raw_root)
    if not root.is_absolute() or not root.is_dir():
        raise MeasurementError("ASTER_CGROUP_ROOT must be an absolute path to an existing directory")
    root = root.resolve()
    for filename in ("cgroup.controllers", "cgroup.subtree_control"):
        if not (root / filename).is_file():
            raise MeasurementError(f"{root / filename} is unavailable")
        controls = read_text(root / filename).split()
        for controller in ("cpu", "memory"):
            if controller not in controls:
                raise MeasurementError(
                    f"{controller} controller is not available/enabled for child cgroups in "
                    f"{root / filename}; no host hierarchy is modified automatically"
                )
    host_kind = os.environ.get("ASTER_BENCH_HOST_KIND", "").strip().lower()
    allowed = {"bare-metal", "virtual-machine", "wsl2", "container", "unknown"}
    if host_kind not in allowed:
        raise MeasurementError(
            "set ASTER_BENCH_HOST_KIND explicitly to one of: "
            + ", ".join(sorted(allowed))
            + " (do not label WSL2 as bare-metal)"
        )
    os.environ["ASTER_CGROUP_ROOT"] = str(root)
    return root, host_kind


def os_release() -> str:
    path = Path("/etc/os-release")
    if not path.exists():
        return "unavailable"
    values = [
        line for line in path.read_text(encoding="utf-8").splitlines()
        if line.startswith(("PRETTY_NAME=", "VERSION_ID="))
    ]
    return ", ".join(values) or "unavailable"


def command_output(command: list[str]) -> str:
    completed = subprocess.run(command, text=True, capture_output=True, check=False)
    return (completed.stdout or completed.stderr).strip().replace("\n", "; ")


def metadata(root: Path, host_kind: str, repeats: int, build_repeats: int, oom_repeats: int) -> dict[str, str]:
    cpu = "unavailable"
    cpuinfo = Path("/proc/cpuinfo")
    if cpuinfo.exists():
        for line in cpuinfo.read_text(encoding="utf-8", errors="replace").splitlines():
            if line.startswith(("model name", "Hardware")):
                cpu = line.split(":", 1)[-1].strip()
                break
    return {
        "timestamp_utc": utc_now(),
        "commit": command_output(["git", "rev-parse", "HEAD"]),
        "working_tree_clean_at_start": "yes" if not command_output(["git", "status", "--porcelain"]) else "no",
        "go_version": command_output(["go", "version"]),
        "go_env": command_output(["go", "env", "GOOS", "GOARCH", "CGO_ENABLED", "GOTOOLCHAIN", "GOFLAGS"]),
        "os_distribution": os_release(),
        "kernel": platform.release(),
        "architecture": platform.machine(),
        "cpu": cpu,
        "logical_cpus": str(os.cpu_count() or "unknown"),
        "host_kind": host_kind,
        "cgroup_root": str(root),
        "cgroup_controllers": read_text(root / "cgroup.controllers"),
        "cgroup_subtree_control": read_text(root / "cgroup.subtree_control"),
        "memory_limit": os.environ.get("ASTER_BENCH_MEMORY_LIMIT", DEFAULT_MEMORY_LIMIT),
        "oom_memory_limit": os.environ.get("ASTER_BENCH_OOM_MEMORY_LIMIT", DEFAULT_OOM_MEMORY_LIMIT),
        "oom_repeats": str(oom_repeats),
        "cpu_rate": os.environ.get("ASTER_BENCH_CPU_RATE", DEFAULT_CPU_RATE),
        "cpu_time_limit": os.environ.get("ASTER_BENCH_CPU_TIME_LIMIT", CPU_TIME_LIMIT),
        "comparison_repeats": str(repeats),
        "build_repeats": str(build_repeats),
        "process_tree_sampling_interval": f"{POLL_SECONDS * 1000:g}ms",
        "cgroup_cpu_poll_interval": CPU_POLL_INTERVAL,
        "cache_condition": "warm toolchain cache; no cold-cache phase is measured",
    }


def process_rss_bytes(pid: int) -> int:
    try:
        for line in Path(f"/proc/{pid}/status").read_text(encoding="utf-8").splitlines():
            if line.startswith("VmRSS:"):
                return int(line.split()[1]) * 1024
    except (OSError, ValueError, IndexError):
        pass
    return 0


def process_tree_pids(root_pid: int) -> set[int]:
    seen: set[int] = set()
    pending = [root_pid]
    while pending:
        pid = pending.pop()
        if pid in seen or not Path(f"/proc/{pid}").exists():
            continue
        seen.add(pid)
        for child_file in Path(f"/proc/{pid}/task").glob("*/children"):
            try:
                pending.extend(int(item) for item in child_file.read_text().split())
            except (OSError, ValueError):
                continue
    return seen


class RunMonitor(threading.Thread):
    def __init__(self, pid: int, root: Path, stop: threading.Event, old_cgroups: set[str]) -> None:
        super().__init__(daemon=True)
        self.pid = pid
        self.root = root
        self.stop_event = stop
        self.old_cgroups = old_cgroups
        self.peak_tree_rss = 0
        self.peak_cgroup_memory = 0
        self.saw_cgroup_peak = False

    def sample(self) -> None:
        tree_rss = sum(process_rss_bytes(pid) for pid in process_tree_pids(self.pid))
        self.peak_tree_rss = max(self.peak_tree_rss, tree_rss)
        try:
            candidates = list(self.root.glob("aster-run-*"))
        except OSError:
            candidates = []
        for cgroup in candidates:
            if cgroup.name in self.old_cgroups:
                continue
            try:
                value = int((cgroup / "memory.peak").read_text(encoding="ascii").strip())
            except (OSError, ValueError):
                continue
            self.peak_cgroup_memory = max(self.peak_cgroup_memory, value)
            self.saw_cgroup_peak = True

    def run(self) -> None:
        while not self.stop_event.is_set():
            self.sample()
            self.stop_event.wait(POLL_SECONDS)
        self.sample()


def json_cell(value: str) -> str:
    return json.dumps(value, ensure_ascii=False)


def run_command(
    command: list[str],
    root: Path,
    records: list[dict[str, Any]],
    *,
    kind: str,
    workload: str,
    profile: str,
    repetition: int,
    expected_output: str | None = None,
    expect_cpu_timeout: bool = False,
    expect_memory_oom: bool = False,
) -> dict[str, Any]:
    before = {item.name for item in root.glob("aster-run-*")}
    started = time.monotonic_ns()
    proc = subprocess.Popen(
        command,
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        start_new_session=True,
    )
    stop = threading.Event()
    monitor = RunMonitor(proc.pid, root, stop, before)
    monitor.start()
    timed_out = False
    try:
        stdout, stderr = proc.communicate(timeout=330)
    except subprocess.TimeoutExpired:
        timed_out = True
        try:
            os.killpg(proc.pid, signal.SIGTERM)
            proc.wait(timeout=3)
        except (ProcessLookupError, subprocess.TimeoutExpired):
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        stdout, stderr = proc.communicate()
    finally:
        stop.set()
        monitor.join(timeout=2)
    lingering = sorted({item.name for item in root.glob("aster-run-*")} - before)
    cleanup_status = "clean" if not lingering else "residual:" + ",".join(lingering)
    wall_ms = (time.monotonic_ns() - started) / 1_000_000
    exit_code = proc.returncode
    outcome = "timeout" if timed_out else ("success" if exit_code == 0 else "nonzero")
    row: dict[str, Any] = {
        "kind": kind,
        "workload": workload,
        "profile": profile,
        "repetition": repetition,
        "timestamp_utc": utc_now(),
        "exit_code": exit_code,
        "outcome": outcome,
        "termination_classification": "normal",
        "cgroup_cleanup_status": cleanup_status,
        "wall_ms": f"{wall_ms:.3f}",
        "sampled_tree_rss_bytes": monitor.peak_tree_rss or "",
        "cgroup_memory_peak_bytes": monitor.peak_cgroup_memory if monitor.saw_cgroup_peak else "",
        "binary_bytes": "",
        "cpu_budget_usec": "",
        "cpu_observed_usec": "",
        "cpu_overshoot_usec": "",
        "cpu_poll_interval": CPU_POLL_INTERVAL if expect_cpu_timeout else "",
        "stdout_json": json_cell(stdout),
        "stderr_json": json_cell(stderr),
    }
    records.append(row)

    if cleanup_status != "clean":
        raise MeasurementError(f"cgroup cleanup failed after {kind}/{workload}/{profile}: {cleanup_status}")
    if timed_out:
        raise MeasurementError(f"{kind}/{workload}/{profile} exceeded 330 seconds")
    if expected_output is not None:
        if exit_code != 0:
            raise MeasurementError(
                f"{kind}/{workload}/{profile} exited {exit_code}; stderr={stderr.strip()!r}"
            )
        if stdout.strip() != expected_output:
            raise MeasurementError(
                f"{kind}/{workload}/{profile} output {stdout.strip()!r}, expected {expected_output!r}"
            )
    if expect_cpu_timeout:
        fields = re.search(
            r"budget_usec=(\d+).*?observed_usec=(\d+).*?overshoot_usec=(\d+).*?sample=post-termination",
            stderr,
            re.DOTALL,
        )
        classified = "RUN019" in stderr or "cumulative CPU-time budget exceeded" in stderr
        if exit_code == 0 or not classified:
            raise MeasurementError(
                "CPU-time termination was not classified as RUN019; "
                f"exit={exit_code}, stderr={stderr.strip()!r}"
            )
        if not fields:
            raise MeasurementError(
                "CPU-time termination did not include a post-termination accounting sample; "
                f"stderr={stderr.strip()!r}"
            )
        row["cpu_budget_usec"], row["cpu_observed_usec"], row["cpu_overshoot_usec"] = fields.groups()
        if int(row["cpu_observed_usec"]) < int(row["cpu_budget_usec"]):
            raise MeasurementError("post-termination CPU usage is below the configured budget")
        if int(row["cpu_overshoot_usec"]) != int(row["cpu_observed_usec"]) - int(row["cpu_budget_usec"]):
            raise MeasurementError("reported CPU-time overshoot does not match observed minus budget")
        row["outcome"] = "cpu-time-limit"
        row["termination_classification"] = "RUN019"
    if expect_memory_oom:
        classified = "RUN018" in stderr or "process memory limit exceeded" in stderr
        if exit_code == 0 or not classified:
            raise MeasurementError(
                "memory-limit OOM was not classified as RUN018; "
                f"exit={exit_code}, stderr={stderr.strip()!r}"
            )
        row["outcome"] = "memory-limit-oom"
        row["termination_classification"] = "RUN018"
    return row


def percentile95(values: list[float]) -> float | None:
    if not values:
        return None
    values = sorted(values)
    return values[max(0, math.ceil(0.95 * len(values)) - 1)]


def summarize(values: list[float]) -> str:
    if not values:
        return "unavailable"
    median = statistics.median(values)
    p95 = percentile95(values)
    return f"n={len(values)}, median={median:.3f}, p95={p95:.3f}, max={max(values):.3f}"


def numeric_values(rows: list[dict[str, Any]], field: str) -> list[float]:
    result: list[float] = []
    for row in rows:
        value = row.get(field, "")
        if value is None or value == "":
            continue
        try:
            result.append(float(value))
        except (ValueError, TypeError):
            continue
    return result


def write_reports(
    markdown_path: Path,
    records: list[dict[str, Any]],
    info: dict[str, str],
    errors: list[str],
) -> Path:
    markdown_path.parent.mkdir(parents=True, exist_ok=True)
    tsv_path = markdown_path.with_suffix(".tsv")
    with tsv_path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=RUN_FIELDS, delimiter="\t", extrasaction="ignore")
        writer.writeheader()
        writer.writerows(records)

    lines = [
        "# Aster Native resource-isolation benchmark",
        "",
        f"Generated at UTC: {info['timestamp_utc']}",
        "",
        "## Environment and configuration",
        "",
    ]
    lines.extend(f"- {key}: {value}" for key, value in info.items())
    lines.extend(
        [
            "",
            "## End-to-end CLI comparison",
            "",
            "Wall time is measured around aster run; it includes source analysis, Native code generation, the Go build, and execution. Baseline and constrained commands use the same source and are interleaved to reduce order bias.",
            "",
            "| Workload | Profile | Wall time (ms) | Sampled process-tree RSS (bytes) | cgroup memory.peak (bytes) |",
            "|---|---|---:|---:|---:|",
        ]
    )
    comparison = [
        row for row in records
        if row["kind"] == "run" and row["workload"] in ("launch-heavy", "cpu-bound", "memory-bounded")
    ]
    for workload in ("launch-heavy", "cpu-bound", "memory-bounded"):
        for profile in ("baseline", "constrained"):
            group = [
                row for row in comparison
                if row["workload"] == workload and row["profile"] == profile and row["outcome"] == "success"
            ]
            wall = numeric_values(group, "wall_ms")
            tree = numeric_values(group, "sampled_tree_rss_bytes")
            cgroup = numeric_values(group, "cgroup_memory_peak_bytes")
            lines.append(
                f"| {workload} | {profile} | {summarize(wall)} | {summarize(tree)} | {summarize(cgroup)} |"
            )

    lines.extend(
        [
            "",
            "Process-tree RSS is sampled from /proc and sums RSS values for visible descendants. It is approximate, may double-count shared pages, and can miss short-lived peaks. The cgroup memory.peak column is sampled from the kernel high-water counter while the Aster cgroup exists. Baseline runs have no Aster cgroup and therefore have no cgroup memory.peak value; the two accounting boundaries are not interchangeable.",
            "",
            "## Separate Native build measurements",
            "",
            "aster build is timed independently for each workload. These samples characterize a comparable build phase but are not subtracted from end-to-end aster run timings: the run path performs its own analysis, code generation, temporary build, and execution, so subtraction would not be a valid runtime-only estimate.",
            "",
            "| Workload | Independent aster build wall time (ms) | Median binary size (bytes) |",
            "|---|---:|---:|",
        ]
    )
    for workload in WORKLOADS:
        group = [
            row for row in records
            if row["kind"] == "build" and row["workload"] == workload and row["outcome"] == "success"
        ]
        sizes = numeric_values(group, "binary_bytes")
        lines.append(
            f"| {workload} | {summarize(numeric_values(group, 'wall_ms'))} | "
            f"{statistics.median(sizes) if sizes else 'unavailable'} |"
        )

    terminations = [
        row for row in records
        if row["kind"] == "run" and row["workload"] == "cpu-time-termination" and row["profile"] == "constrained"
    ]
    lines.extend(
        [
            "",
            "## CPU-time termination and overshoot",
            "",
            f"Configured budget: {info['cpu_time_limit']}. CPU accounting poll interval: {CPU_POLL_INTERVAL}. Each accepted sample must exit with RUN019 and include the final sample=post-termination accounting data.",
            "",
            "| Metric | Observed values (microseconds) |",
            "|---|---:|",
        ]
    )
    for label, field in (
        ("Budget", "cpu_budget_usec"),
        ("Post-termination observed CPU time", "cpu_observed_usec"),
        ("Observed overshoot", "cpu_overshoot_usec"),
    ):
        lines.append(f"| {label} | {summarize(numeric_values(terminations, field))} |")

    oom_rows = [
        row for row in records
        if row["kind"] == "run" and row["workload"] == "memory-oom" and row["profile"] == "constrained"
    ]
    oom_classified = [
        row for row in oom_rows
        if row["outcome"] == "memory-limit-oom"
        and row["termination_classification"] == "RUN018"
        and row["cgroup_cleanup_status"] == "clean"
    ]
    lines.extend(
        [
            "",
            "## Memory-limit OOM classification and cleanup",
            "",
            f"Configured cgroup memory limit: {info['oom_memory_limit']}. The OOM workload retains five independent 16 MiB strings, intentionally exceeding this limit. Each run must be classified as RUN018 and leave no Aster cgroup behind.",
            "",
            "| Check | Result |",
            "|---|---:|",
            f"| RUN018 classification and clean cgroup cleanup | {len(oom_classified)}/{len(oom_rows)} |",
            f"| Recorded OOM samples | {len(oom_rows)} |",
            "",
            "## Overshoot interpretation",
            "",
            "The maximum observed overshoot is the sample maximum for this host and these runs, not a kernel-independent or workload-independent upper bound. Exact supervisor setup/cleanup duration is not exported by the current CLI, so it is reported as unavailable rather than inferred from total CLI latency.",
            "",
            "## Raw data",
            "",
            f"- Machine-readable raw samples: {tsv_path.name}",
            "- The TSV includes exit classification, raw stdout/stderr JSON strings, wall-time samples, sampled process-tree RSS, available cgroup memory peaks, CPU budget/observed/overshoot fields, and cgroup cleanup status.",
            "",
        ]
    )
    if errors:
        lines.extend(["## Errors / incomplete run", ""])
        lines.extend(f"- {error}" for error in errors)
        lines.extend(["", "This report is partial and must not be treated as a complete acceptance result.", ""])
    else:
        lines.extend(
            [
                "## Interpretation and limits",
                "",
                "- Measurements describe only the recorded host, kernel, Go toolchain, configuration, and commit. WSL2 evidence is not bare-metal Linux evidence.",
                "- The same source and warmed Go toolchain cache are used for baseline and constrained cases. Cold-cache performance is not measured.",
                "- Constrained mode is opt-in and depends on a delegated cgroup v2 hierarchy. This benchmark does not establish a general Linux guarantee or a security sandbox.",
                "- No fixed performance threshold is used. The largest finite-sample CPU-time overshoot is an empirical observation, not a hard guarantee.",
                "- The memory workload retains two 16 MiB ASCII strings and requests a 256 MiB cgroup memory limit by default. It tests only the recorded configuration.",
                "",
            ]
        )
    markdown_path.write_text("\n".join(lines), encoding="utf-8")
    return tsv_path


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Measure Native CLI overhead and CPU-time overshoot on a delegated Linux cgroup v2."
    )
    parser.add_argument(
        "output",
        nargs="?",
        help="Markdown report path (default: bench/results/resource-isolation-<UTC timestamp>.md)",
    )
    args = parser.parse_args()

    try:
        root, host_kind = preflight()
        repeats = positive_int_env("ASTER_BENCH_REPEATS", 30)
        build_repeats = positive_int_env("ASTER_BENCH_BUILD_REPEATS", 5)
        oom_repeats = positive_int_env("ASTER_BENCH_OOM_REPEATS", 3)
        memory_limit = os.environ.get("ASTER_BENCH_MEMORY_LIMIT", DEFAULT_MEMORY_LIMIT)
        oom_memory_limit = os.environ.get("ASTER_BENCH_OOM_MEMORY_LIMIT", DEFAULT_OOM_MEMORY_LIMIT)
        cpu_rate = os.environ.get("ASTER_BENCH_CPU_RATE", DEFAULT_CPU_RATE)
        cpu_time_limit = os.environ.get("ASTER_BENCH_CPU_TIME_LIMIT", CPU_TIME_LIMIT)
        info = metadata(root, host_kind, repeats, build_repeats, oom_repeats)
        info["memory_limit"] = memory_limit
        info["oom_memory_limit"] = oom_memory_limit
        info["oom_repeats"] = str(oom_repeats)
        info["cpu_rate"] = cpu_rate
        info["cpu_time_limit"] = cpu_time_limit
        stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        report_path = Path(args.output) if args.output else ROOT / "bench" / "results" / f"resource-isolation-{stamp}.md"
        if not report_path.is_absolute():
            report_path = ROOT / report_path
        report_path = report_path.resolve()
    except (MeasurementError, OSError) as exc:
        print(f"benchmark preflight failed: {exc}", file=sys.stderr)
        return 2

    records: list[dict[str, Any]] = []
    errors: list[str] = []
    try:
        with tempfile.TemporaryDirectory(prefix="aster-resource-bench-") as tmp_name:
            tmp = Path(tmp_name)
            sources = tmp / "src"
            binaries = tmp / "bin"
            sources.mkdir()
            binaries.mkdir()
            for workload, definition in WORKLOADS.items():
                (sources / f"{workload}.ast").write_text(definition["source"], encoding="utf-8")

            cli = tmp / "aster"
            started = time.monotonic_ns()
            built_cli = subprocess.run(
                ["go", "build", "-trimpath", "-o", str(cli), "./cmd/aster"],
                cwd=ROOT,
                text=True,
                capture_output=True,
                check=False,
            )
            info["harness_cli_build_ms"] = f"{(time.monotonic_ns() - started) / 1_000_000:.3f}"
            if built_cli.returncode != 0:
                raise MeasurementError(f"failed to build Aster CLI: {built_cli.stderr.strip()}")

            for workload in WORKLOADS:
                source = sources / f"{workload}.ast"
                for repetition in range(1, build_repeats + 1):
                    output_binary = binaries / f"{workload}-{repetition}"
                    row = run_command(
                        [str(cli), "build", "-o", str(output_binary), str(source)],
                        root,
                        records,
                        kind="build",
                        workload=workload,
                        profile="standalone-build",
                        repetition=repetition,
                    )
                    if not output_binary.is_file():
                        raise MeasurementError(f"aster build did not create expected binary for {workload}")
                    row["binary_bytes"] = output_binary.stat().st_size

            comparison_workloads = ("launch-heavy", "cpu-bound", "memory-bounded")
            for repetition in range(1, repeats + 1):
                for workload in comparison_workloads:
                    profiles = ("baseline", "constrained") if repetition % 2 else ("constrained", "baseline")
                    for profile in profiles:
                        source = sources / f"{workload}.ast"
                        if profile == "baseline":
                            command = [str(cli), "run", "--engine", "native", str(source)]
                        else:
                            command = [
                                str(cli), "run", "--engine", "native",
                                "--resource-profile", "constrained",
                                "--memory-limit", memory_limit,
                                "--cpu-rate", cpu_rate,
                                str(source),
                            ]
                        run_command(
                            command,
                            root,
                            records,
                            kind="run",
                            workload=workload,
                            profile=profile,
                            repetition=repetition,
                            expected_output=WORKLOADS[workload]["expected"],
                        )

            source = sources / "cpu-time-termination.ast"
            for repetition in range(1, repeats + 1):
                run_command(
                    [
                        str(cli), "run", "--engine", "native",
                        "--resource-profile", "constrained",
                        "--cpu-time-limit", cpu_time_limit,
                        str(source),
                    ],
                    root,
                    records,
                    kind="run",
                    workload="cpu-time-termination",
                    profile="constrained",
                    repetition=repetition,
                    expect_cpu_timeout=True,
                )

            oom_source = sources / "memory-oom.ast"
            for repetition in range(1, oom_repeats + 1):
                run_command(
                    [
                        str(cli), "run", "--engine", "native",
                        "--resource-profile", "constrained",
                        "--memory-limit", oom_memory_limit,
                        str(oom_source),
                    ],
                    root,
                    records,
                    kind="run",
                    workload="memory-oom",
                    profile="constrained",
                    repetition=repetition,
                    expect_memory_oom=True,
                )
    except (MeasurementError, OSError, subprocess.SubprocessError) as exc:
        errors.append(str(exc))
    except Exception as exc:
        errors.append(f"unexpected harness error: {type(exc).__name__}: {exc}")

    try:
        tsv_path = write_reports(report_path, records, info, errors)
    except OSError as exc:
        print(f"failed to write benchmark report: {exc}", file=sys.stderr)
        return 2

    print(f"Markdown report: {report_path}")
    print(f"Raw TSV samples: {tsv_path}")
    if errors:
        for error in errors:
            print(f"benchmark incomplete: {error}", file=sys.stderr)
        return 1

    expected_run_count = repeats * 7 + oom_repeats
    actual_run_count = sum(1 for row in records if row["kind"] == "run")
    if actual_run_count != expected_run_count:
        print(
            f"benchmark incomplete: recorded {actual_run_count} run samples, expected {expected_run_count}",
            file=sys.stderr,
        )
        return 1
    if repeats < 30:
        print(
            f"warning: only {repeats} repetitions requested; acceptance target is at least 30",
            file=sys.stderr,
        )
    print(f"Completed {actual_run_count} Native run samples and {build_repeats * len(WORKLOADS)} build samples.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
