# v3 Recovery Request-ID Diagnostic

Status: diagnostic record only. This is not a pilot acceptance report and
does not authorize a live run.

## Scope

- Run: `run-1788136436747506769-1`
- Frozen checkout: `09ad57b`
- Symptom observed in the v3 live-pilot reproduction: after recovery rotated
  the Builder prompt from the original `:prompt` ID to `:repair-1`, the Agent
  completed new work but returned an Evidence envelope carrying the original
  request ID. Strict request validation rejected that envelope.

## Diagnosis

The recovery path persisted the rotated request ID but its continuation packet
did not explicitly invalidate earlier Evidence envelopes or repeat the exact
new ID as an Evidence requirement. An Agent could therefore retain the prior
ID while continuing the existing work. The validator behavior is correct and
remains strict.

## Expected invariant

Every recovery or repair packet must direct the Agent to preserve the current
commit and unfinished work, avoid repeating completed work, and emit a new
Evidence envelope only after exact verification. The packet must quote and
repeat the current request ID and, when known, say not to reuse the prior ID.
The durable task state records the prior request ID for this diagnostic trail.

## Safe evidence boundary

This record contains identifiers, the observed state transition, and the
diagnosis only. It intentionally excludes terminal transcripts, Evidence
payloads, credentials, tokens, repository contents, and provider response
data. Any future pilot run must use a newly selected frozen checkout and the
operator's approved secret-handling procedure.
