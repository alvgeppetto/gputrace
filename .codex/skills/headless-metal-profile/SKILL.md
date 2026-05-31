---
name: headless-metal-profile
description: Use the project-local gputrace CLI to collect non-GUI Apple Metal timing rows with xctrace, especially for targets that need attach-after-launch synchronization.
metadata:
  short-description: Headless Apple Metal profiling
---

# Headless Metal Profile

Use this skill when an agent needs Metal GPU timing evidence from the local
`gputrace` project without opening Xcode or Instruments.

## Safety Gate

Before every heavy capture, export, or replay, run:

```sh
df -h <output-volume> <system-data-volume>
memory_pressure -Q
ps -axo pid=,comm= | egrep '/(xctrace|gputrace|MTLReplayer|gt_replay_service_probe_helper|xctrace_streamdata_helper)$|GPUToolsReplayService' || true
```

Do not start a capture if the output volume or system data volume is below the
project's active free-space gate, if memory pressure is below the active gate,
or if profiler/replay helper processes are already running.

## Preferred Command Shape

For targets with startup/setup work before the GPU phase, prefer
`--attach-launched` plus `--attach-after-file`:

```sh
OUT=<output-volume>/gputrace-results/run-001
TMPROOT=<output-volume>/gputrace-results/tmp/run-001
mkdir -p "$OUT" "$TMPROOT"

TMPDIR="$TMPROOT" TEMP="$TMPROOT" TMP="$TMPROOT" \
gputrace headless-profile \
  --json \
  --attach-launched \
  --attach-after-file "$OUT/ready.json" \
  --attach-wait 120s \
  --out-dir "$OUT" \
  --trace-name capture.trace \
  --process target-process-name \
  --time-limit 10s \
  --timeout 2m \
  --min-out-dir-free-gib 24 \
  --min-memory-free-percent 10 \
  -- /path/to/target --write-ready-file "$OUT/ready.json" \
  > "$OUT/headless-profile.json" \
  2> "$OUT/headless-profile.stderr"
```

The target process must create the `--attach-after-file` path shortly before the
GPU work to be profiled. If no ready-file hook exists, omit
`--attach-after-file` and set `--attach-wait` long enough for the process to
start.

## Success Criteria

A timing capture is usable only when:

- `headless-profile.json` contains `profile.timing_claims_allowed=true`
- `profile.toc.exportable=true`
- `profile/xctrace_metal-gpu-intervals.xml` exists
- `profile/xctrace-profile.gpuprofiler_raw/streamData` exists
- `profile.streamData.rows_encoded > 0`

Treat `profile.counter_claims_allowed=false` as expected unless a separate
counter pipeline has proved route-attributed counter samples.

## Failure Triage

If `xctrace-profile` reports `toc.exportable=false`, the trace is not
exportable by `xctrace`; do not treat missing `metal-gpu-intervals` as a parser
bug. Use the JSON `reason`, `toc.export.stderr_preview`, record command, and
record signal to decide whether to change capture shape.

If `toc.exportable=true` but timing rows are empty, inspect
`profile.toc.schemas`, table row counts, target process spelling, and exported
Metal interval labels.
