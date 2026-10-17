# Review evidence receipts

Use this optional collector only when a user-supplied Codex/OpenCode event artifact would otherwise require repeated manual extraction:

```text
python3 scripts/summarize_review_evidence.py \
  --format auto \
  /path/to/one-events.jsonl
```

Supported inputs are Codex JSONL, OpenCode JSONL, and an OpenCode session export JSON file. The command reads exactly the one named regular file, writes a compact JSON summary to stdout, and performs no model call, network request, database/session lookup, artifact write, or recursive scan. Select a format explicitly when conservative auto-detection cannot identify it.

The summary reports only explicit native metadata observed in the supplied artifact: source/offset, parent and child IDs, an explicit `td_reviewer` role, lifecycle state, and the location of a native result field. It is not an authenticated runtime attestation. It omits prompts, tool output, result text, and other raw content, and never parses strings nested inside those fields as events. A malformed, truncated, or unsupported artifact produces an error/unknown result rather than a receipt.

Treat `data_sufficiency` as a receipt-collection bound, not a verdict. Parent prose claiming review, an outer successful turn, an empty wait, a non-review child, or a task without an explicit child ID cannot prove independent review. A missing/unknown role or unsupported trace is unverified, not proof that a review did not occur. Even a sufficient completed receipt does not prove which plan/code revision was reviewed or whether the review was correct. The reviewer must still compare the fixed candidate, acceptance criteria, checks, and substantive result under this Skill.
