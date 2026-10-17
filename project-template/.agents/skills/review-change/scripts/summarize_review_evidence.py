#!/usr/bin/env python3
"""Summarize native review-task metadata without copying review content."""

from __future__ import annotations

import argparse
import json
import stat
import sys
from pathlib import Path
from typing import Any


SCHEMA_VERSION = "1"
FORMATS = ("auto", "codex-jsonl", "opencode-jsonl", "opencode-export")
LIFECYCLE_STATUSES = {"started", "running", "completed", "failed", "unknown"}
REVIEW_ROLE = "td_reviewer"

CODEX_SPAWN_TYPES = {"collab.spawn", "collaboration.spawn", "task.spawn", "child.spawn"}
CODEX_WAIT_TYPES = {
    "collab.wait",
    "collaboration.wait",
    "collab.completed",
    "collaboration.completed",
    "task.completed",
    "child.completed",
}


def _json_pointer(path: list[str | int]) -> str:
    escaped = []
    for item in path:
        value = str(item).replace("~", "~0").replace("/", "~1")
        escaped.append(value)
    return "/" + "/".join(escaped)


def _status(value: Any, default: str = "unknown") -> str:
    if isinstance(value, str) and value.lower() in LIFECYCLE_STATUSES:
        return value.lower()
    return default


def _role(event: dict[str, Any]) -> str | None:
    for key in ("explicit_role", "role", "agent_role"):
        value = event.get(key)
        if isinstance(value, str):
            return value
    return None


def _child_id(event: dict[str, Any]) -> str | None:
    for key in ("child_id", "childId", "child"):
        value = event.get(key)
        if isinstance(value, str) and value:
            return value
    return None


def _result_location(event: dict[str, Any]) -> str | None:
    for key in ("result_location", "resultLocation"):
        value = event.get(key)
        if isinstance(value, str) and value:
            return value
    return None


def _codex_event_type(event: dict[str, Any]) -> str | None:
    event_type = event.get("type")
    if not isinstance(event_type, str):
        return None
    event_type = event_type.lower()
    if event_type in CODEX_SPAWN_TYPES | CODEX_WAIT_TYPES or event_type.startswith("turn."):
        return event_type
    if event_type in {"collab", "collaboration"}:
        action = event.get("action") or event.get("event")
        if isinstance(action, str):
            candidate = f"{event_type}.{action.lower()}"
            if candidate in CODEX_SPAWN_TYPES | CODEX_WAIT_TYPES:
                return candidate
    return event_type


def _base_summary(source_file: str, format_name: str) -> dict[str, Any]:
    return {
        "schema_version": SCHEMA_VERSION,
        "source_file": source_file,
        "format": format_name,
        "parse_status": "ok",
        "parent": {"lifecycle_status": "unknown", "evidence_offsets": []},
        "review_receipts": [],
        "outer_status": "unknown",
        "data_sufficiency": "unknown",
        "limitations": [],
    }


def _error_summary(source_file: str, format_name: str, limitation: str) -> dict[str, Any]:
    summary = _base_summary(source_file, format_name)
    summary["parse_status"] = "error"
    summary["data_sufficiency"] = "unknown"
    summary["limitations"] = [limitation]
    return summary


def _finalize(summary: dict[str, Any], limitations: set[str]) -> dict[str, Any]:
    receipts = summary["review_receipts"]
    receipts.sort(key=lambda item: (item["child_id"], item["explicit_role"], item["lifecycle_status"]))
    for receipt in receipts:
        receipt["evidence_offsets"] = _sorted_offsets(receipt["evidence_offsets"])
    summary["limitations"] = sorted(limitations)
    if summary["parse_status"] == "error":
        summary["data_sufficiency"] = "unknown"
    elif any(
        receipt["lifecycle_status"] == "completed" and receipt["result_location"] is not None
        for receipt in receipts
    ):
        summary["data_sufficiency"] = "sufficient"
    else:
        summary["data_sufficiency"] = "partial"
    summary["parent"]["evidence_offsets"] = _sorted_offsets(summary["parent"]["evidence_offsets"])
    return summary


def _sorted_offsets(offsets: list[int | str]) -> list[int | str]:
    return sorted(set(offsets), key=lambda value: (0, value) if isinstance(value, int) else (1, str(value)))


def _codex_summary(source_file: str, events: list[dict[str, Any]], format_name: str) -> dict[str, Any]:
    summary = _base_summary(source_file, format_name)
    limitations: set[str] = set()
    records: dict[str, dict[str, Any]] = {}

    for line_number, event in enumerate(events, start=1):
        lowered_type = _codex_event_type(event)
        if lowered_type is None:
            continue
        if lowered_type == "turn.started":
            summary["outer_status"] = "started"
            summary["parent"]["lifecycle_status"] = "started"
            summary["parent"]["evidence_offsets"].append(line_number)
        elif lowered_type == "turn.running":
            summary["outer_status"] = "running"
            summary["parent"]["lifecycle_status"] = "running"
            summary["parent"]["evidence_offsets"].append(line_number)
        elif lowered_type == "turn.completed":
            summary["outer_status"] = "completed"
            summary["parent"]["lifecycle_status"] = "completed"
            summary["parent"]["evidence_offsets"].append(line_number)
        elif lowered_type == "turn.failed":
            summary["outer_status"] = "failed"
            summary["parent"]["lifecycle_status"] = "failed"
            summary["parent"]["evidence_offsets"].append(line_number)

        if lowered_type in CODEX_SPAWN_TYPES:
            child_id = _child_id(event)
            role = _role(event)
            if child_id is None:
                limitations.add("child_id_not_observed")
                continue
            if role != REVIEW_ROLE:
                if role is None:
                    limitations.add("reviewer_role_not_observed")
                else:
                    limitations.add("non_review_task_observed")
                continue
            records[child_id] = {
                "child_id": child_id,
                "explicit_role": REVIEW_ROLE,
                "lifecycle_status": _status(event.get("status"), "started"),
                "result_location": None,
                "evidence_offsets": [line_number],
            }
            continue

        if lowered_type in CODEX_WAIT_TYPES:
            child_id = _child_id(event)
            role = _role(event)
            if child_id is None:
                limitations.add("child_id_not_observed")
                continue
            if child_id not in records:
                limitations.add("serialized_spawn_not_observed")
                continue
            if role is not None and role != REVIEW_ROLE:
                limitations.add("non_review_task_observed")
                continue
            record = records[child_id]
            record["evidence_offsets"].append(line_number)
            lifecycle = _status(event.get("status"))
            location = _result_location(event)
            if lifecycle == "completed" and location is None:
                record["lifecycle_status"] = "unknown"
                limitations.add("result_location_not_observed")
            else:
                record["lifecycle_status"] = lifecycle
                record["result_location"] = location
            if lifecycle in {"failed", "running"}:
                limitations.add("task_not_completed")

    summary["review_receipts"] = list(records.values())
    if not records:
        limitations.add("reviewer_receipt_not_observed")
    if summary["outer_status"] == "completed" and not records:
        limitations.add("outer_completion_is_not_review_proof")
    return _finalize(summary, limitations)


def _state_from_event(event: dict[str, Any]) -> list[tuple[dict[str, Any], str]]:
    states: list[tuple[dict[str, Any], str]] = []
    state = event.get("state")
    if isinstance(state, dict):
        states.append((state, "/state"))
    part = event.get("part")
    if isinstance(part, dict) and isinstance(part.get("state"), dict):
        states.append((part["state"], "/part/state"))
    return states


def _opencode_state_receipt(
    state: dict[str, Any], base_pointer: str, evidence_offset: int | str
) -> tuple[dict[str, Any] | None, set[str]]:
    limitations: set[str] = set()
    task_input = state.get("input")
    metadata = state.get("metadata")
    role = task_input.get("subagent_type") if isinstance(task_input, dict) else None
    child_id = metadata.get("sessionId") if isinstance(metadata, dict) else None
    if role != REVIEW_ROLE:
        limitations.add("non_review_task_observed" if role is not None else "reviewer_role_not_observed")
        return None, limitations
    if not isinstance(child_id, str) or not child_id:
        limitations.add("child_id_not_observed")
        return None, limitations

    lifecycle = _status(state.get("status"))
    location = None
    if lifecycle == "completed":
        for key in ("result", "output"):
            if key in state:
                location = f"{base_pointer}/{key}"
                break
        if location is None:
            lifecycle = "unknown"
            limitations.add("result_location_not_observed")
    if lifecycle in {"failed", "running"}:
        limitations.add("task_not_completed")
    return (
        {
            "child_id": child_id,
            "explicit_role": REVIEW_ROLE,
            "lifecycle_status": lifecycle,
            "result_location": location,
            "evidence_offsets": [evidence_offset],
        },
        limitations,
    )


def _opencode_summary(
    source_file: str,
    events: list[dict[str, Any]],
    format_name: str,
    export: bool = False,
) -> dict[str, Any]:
    summary = _base_summary(source_file, format_name)
    limitations: set[str] = set()
    receipts: dict[str, dict[str, Any]] = {}

    if export:
        messages = events[0].get("messages") if events else None
        if not isinstance(messages, list):
            return _error_summary(source_file, format_name, "unsupported_export_structure")
        candidates = []
        for message_index, message in enumerate(messages):
            if not isinstance(message, dict) or not isinstance(message.get("parts"), list):
                continue
            for part_index, part in enumerate(message["parts"]):
                if not isinstance(part, dict) or not isinstance(part.get("state"), dict):
                    continue
                candidates.append((part["state"], f"/messages/{message_index}/parts/{part_index}/state", _json_pointer(["messages", message_index, "parts", part_index, "state"])))
        for state, base_pointer, evidence_pointer in candidates:
            receipt, state_limitations = _opencode_state_receipt(state, base_pointer, evidence_pointer)
            limitations.update(state_limitations)
            if receipt is None:
                continue
            existing = receipts.get(receipt["child_id"])
            if existing is None or receipt["evidence_offsets"][-1] > existing["evidence_offsets"][-1]:
                receipts[receipt["child_id"]] = receipt
    else:
        for line_number, event in enumerate(events, start=1):
            for state, base_pointer in _state_from_event(event):
                receipt, state_limitations = _opencode_state_receipt(state, base_pointer, line_number)
                limitations.update(state_limitations)
                if receipt is None:
                    continue
                existing = receipts.get(receipt["child_id"])
                if existing is None or line_number >= existing["evidence_offsets"][-1]:
                    receipts[receipt["child_id"]] = receipt

    summary["review_receipts"] = list(receipts.values())
    if not receipts:
        limitations.add("reviewer_receipt_not_observed")
    return _finalize(summary, limitations)


def _parse_jsonl(text: str) -> tuple[list[dict[str, Any]] | None, str | None]:
    events: list[dict[str, Any]] = []
    if not text.strip():
        return None, "empty_input"
    for raw_line in text.splitlines():
        if not raw_line.strip():
            continue
        try:
            value = json.loads(raw_line)
        except json.JSONDecodeError:
            return None, "truncated_or_invalid_jsonl"
        if not isinstance(value, dict):
            return None, "unsupported_jsonl_event"
        events.append(value)
    if not events:
        return None, "empty_input"
    return events, None


def _looks_like_codex(events: list[dict[str, Any]]) -> bool:
    return any(
        _codex_event_type(event) in CODEX_SPAWN_TYPES | CODEX_WAIT_TYPES
        or (isinstance(event.get("type"), str) and event["type"].lower().startswith("turn."))
        for event in events
    )


def _looks_like_opencode(events: list[dict[str, Any]]) -> bool:
    return any(_state_from_event(event) for event in events)


def _read_artifact(path: Path) -> tuple[str | None, str | None]:
    try:
        mode = path.stat().st_mode
    except OSError:
        return None, "artifact_unreadable"
    if not stat.S_ISREG(mode):
        return None, "artifact_is_not_regular_file"
    try:
        return path.read_text(encoding="utf-8"), None
    except (OSError, UnicodeDecodeError):
        return None, "artifact_unreadable"


def summarize(source_file: str, requested_format: str) -> tuple[dict[str, Any], int]:
    path = Path(source_file)
    text, read_error = _read_artifact(path)
    if read_error:
        return _error_summary(source_file, requested_format, read_error), 1

    format_name = requested_format
    if requested_format == "opencode-export":
        try:
            document = json.loads(text)
        except json.JSONDecodeError:
            return _error_summary(source_file, requested_format, "invalid_export_json"), 1
        if not isinstance(document, dict) or not isinstance(document.get("messages"), list):
            return _error_summary(source_file, requested_format, "unsupported_export_structure"), 1
        return _opencode_summary(source_file, [document], requested_format, export=True), 0

    if requested_format == "auto":
        try:
            document = json.loads(text)
        except json.JSONDecodeError:
            document = None
        if isinstance(document, dict) and isinstance(document.get("messages"), list):
            return _opencode_summary(source_file, [document], "opencode-export", export=True), 0

    events, parse_error = _parse_jsonl(text)
    if requested_format == "auto":
        if events is not None:
            codex = _looks_like_codex(events)
            opencode = _looks_like_opencode(events)
            if codex and opencode:
                return _error_summary(source_file, "auto", "ambiguous_format"), 1
            if codex:
                format_name = "codex-jsonl"
            elif opencode:
                format_name = "opencode-jsonl"
            else:
                return _error_summary(source_file, "auto", "unsupported_format"), 1
        else:
            return _error_summary(source_file, "auto", parse_error or "invalid_input"), 1
    elif parse_error:
        return _error_summary(source_file, requested_format, parse_error), 1

    if events is None:
        return _error_summary(source_file, format_name, parse_error or "invalid_input"), 1
    if format_name == "codex-jsonl":
        return _codex_summary(source_file, events, format_name), 0
    if format_name == "opencode-jsonl":
        return _opencode_summary(source_file, events, format_name), 0
    return _error_summary(source_file, format_name, "unsupported_format"), 1


def _argument_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Summarize review-task evidence from one artifact")
    parser.add_argument("--format", choices=FORMATS, default="auto")
    parser.add_argument("artifact", help="one regular JSONL or export artifact")
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = _argument_parser()
    try:
        args = parser.parse_args(argv)
    except SystemExit as error:
        if error.code == 0:
            raise
        summary = _error_summary("", "auto", "invalid_arguments")
        sys.stdout.write(json.dumps(summary, sort_keys=True, separators=(",", ":")) + "\n")
        return int(error.code) if isinstance(error.code, int) else 2
    summary, return_code = summarize(args.artifact, args.format)
    sys.stdout.write(json.dumps(summary, sort_keys=True, separators=(",", ":")) + "\n")
    return return_code


if __name__ == "__main__":
    raise SystemExit(main())
