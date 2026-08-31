# Parallel pilot blocker: v2 Reviewer routing

- Run ID: `run-1788135341435506215-1`
- Frozen source SHA: `effb0fa`

## Symptom and root cause

The v2 parallel pilot produced valid strict Reviewer output and persisted the
`review schema sent` receipt. The CLI `resume` path treated that receipt as a
terminal Single-run handoff and stopped without calling the coordinator. A
parallel run must continue through strict `ReviewEvidence`, pull request, CI,
and merge gates; this routing behavior was therefore a pilot blocker.

The fix keeps the schema-receipt terminal handoff for Single-run and legacy
snapshots, while parallel snapshots advance through the coordinator.

## Evidence handling

This note contains no secrets and no raw terminal transcript. The run is
diagnostic rather than gate evidence: it stopped after valid Reviewer output,
before the parallel ReviewEvidence/PR/CI/merge sequence completed. Use the
focused CLI tests and post-fix repository checks for acceptance evidence.
