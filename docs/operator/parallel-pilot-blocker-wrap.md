> **Legacy v1 참고 자료.** 기존 구현·파일럿의 절차와 관찰 기록이며 새 MVP의 실행 계획이나 완료 조건이 아닙니다. [현재 설계](../superpowers/specs/2026-09-07-project-workflow-mvp-design.md)를 먼저 따르십시오.

# Parallel pilot blocker: terminal-wrapped evidence

- Run ID: `run-1788131976915047221-1`
- Frozen source SHA: `fb0fe12`

## Symptom and root cause

The live parallel pilot reached the Builder evidence step, but `ReadEvidence`
rejected the Herdr `recent-unwrapped` response as non-structured JSON. OpenCode
rendered the one-line Evidence envelope at terminal width, inserting physical
line breaks inside JSON strings. In particular, the `commitSha` and the exact
`test "$(cat pkg/alpha/value.txt)" = "alpha-ready"` command were split across
display-indented lines;
the same rendering can split a string immediately beside an escape/backslash.
The Reviewer wrap test in this change is synthetic coverage, not a captured
Reviewer transcript.

Envelope extraction now joins only a continuation observed while its JSON
scanner is inside a quoted string and has exactly the display indentation
established by the first payload line. Missing or mismatched indentation keeps
the newline, so strict JSON decoding rejects it. Newlines outside strings
remain in the payload, and raw payload byte accounting, marker/sidebar guards,
strict decode, credential checks, and malformed-envelope rejection remain
unchanged.

## Evidence handling

This note contains no secrets and no raw terminal transcript. The pilot run is
diagnostic rather than gate evidence: its terminal rendering demonstrated the
Builder transport symptom, but the run predates this parser fix and therefore
cannot prove the corrected parser or the repository verification gate. Use the
focused tests and the post-fix repository checks for that purpose.
