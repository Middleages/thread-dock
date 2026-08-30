# Parallel pilot blocker: terminal-wrapped evidence

- Run ID: `run-1787939711953381828-1`
- Frozen source SHA: `fb0fe12`

## Symptom and root cause

The live parallel pilot reached the Builder evidence step, but `ReadEvidence`
rejected the Herdr `recent-unwrapped` response as non-structured JSON. OpenCode
rendered the one-line Evidence envelope at terminal width, inserting physical
line breaks inside JSON strings. In particular, the `commitSha` and the exact
`test -f pilot-result.txt` command were split across display-indented lines;
the same rendering can split a string immediately beside an escape/backslash.

Envelope extraction now joins only a continuation observed while its JSON
scanner is inside a quoted string, removing only the display indentation
established by the first payload line. Newlines outside strings remain in the
payload, and raw payload byte accounting, marker/sidebar guards, strict decode,
credential checks, and malformed-envelope rejection remain unchanged.

## Evidence handling

This note contains no secrets and no raw terminal transcript. The pilot run is
diagnostic rather than gate evidence: its terminal rendering demonstrated the
transport symptom, but the run predates this parser fix and therefore cannot
prove the corrected parser or the repository verification gate. Use the
focused tests and the post-fix repository checks for that purpose.
