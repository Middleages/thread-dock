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

CODEX_ITEM_TYPES = {"item.started", "item.completed"}


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
    for key in ("agent_type", "explicit_role", "role", "agent_role"):
        value = event.get(key)
        if isinstance(value, str):
            return value
    return None


def _child_id(event: dict[str, Any]) -> str | None:
    for key in ("child_id", "childId", "child", "receiver_thread_id"):
        value = event.get(key)
        if isinstance(value, str) and value:
            return value
    return None


def _receiver_ids(event: dict[str, Any]) -> list[str]:
    receiver_ids = event.get("receiver_thread_ids")
    if isinstance(receiver_ids, list):
        return [value for value in receiver_ids if isinstance(value, str) and value]
    child_id = _child_id(event)
    return [child_id] if child_id is not None else []


def _result_location(event: dict[str, Any]) -> str | None:
    for key in ("result_location", "resultLocation"):
        value = event.get(key)
        if isinstance(value, str) and value:
            return value
    return None


def _non_empty_result(value: Any) -> bool:
    if value is None:
        return False
    if isinstance(value, (str, list, dict, tuple, set)) and not value:
        return False
    return True


def _base_summary(source_file: str, format_name: str) -> dict[str, Any]:
    return {
        "schema_version": SCHEMA_VERSION,
        "source_file": source_file,
        "format": format_name,
        "parse_status": "ok",
        "parent": {"parent_id": None, "lifecycle_status": "unknown", "evidence_offsets": []},
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
        event_type = event.get("type")
        if event_type == "thread.started":
            parent_id = event.get("thread_id")
            if isinstance(parent_id, str) and parent_id:
                summary["parent"]["parent_id"] = parent_id
                summary["parent"]["evidence_offsets"].append(line_number)
            else:
                limitations.add("parent_id_not_observed")
            continue
        if event_type in {"turn.started", "turn.running", "turn.completed", "turn.failed"}:
            summary["outer_status"] = event_type.removeprefix("turn.")
            continue
        if event_type not in CODEX_ITEM_TYPES:
            continue
        item = event.get("item")
        if not isinstance(item, dict) or item.get("type") != "collab_tool_call":
            continue
        tool = item.get("tool")
        if tool not in {"spawn_agent", "wait", "wait_agent"}:
            continue
        role = _role(item)
        receiver_ids = _receiver_ids(item)
        if not receiver_ids:
            limitations.add("child_id_not_observed")
            if tool == "wait_agent":
                limitations.add("empty_receiver_wait")
            continue
        if role != REVIEW_ROLE:
            limitations.add("reviewer_role_not_observed" if role is None else "non_review_task_observed")
            continue

        if tool == "spawn_agent":
            lifecycle = "running" if _status(item.get("status")) == "running" else "started"
        else:
            lifecycle = _status(item.get("status"), "unknown")
        if event_type == "item.completed" and tool in {"wait", "wait_agent"} and "status" not in item:
            lifecycle = "completed"
        location = _result_location(item)
        for child_id in receiver_ids:
            if tool == "spawn_agent":
                records[child_id] = {
                    "child_id": child_id,
                    "explicit_role": REVIEW_ROLE,
                    "lifecycle_status": lifecycle,
                    "result_location": None,
                    "evidence_offsets": [line_number],
                }
                continue
            if child_id not in records:
                limitations.add("serialized_spawn_not_observed")
                continue
            record = records[child_id]
            record["evidence_offsets"].append(line_number)
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
    if summary["parent"]["parent_id"] is None:
        limitations.add("parent_id_not_observed")
    return _finalize(summary, limitations)


def _state_from_event(event: dict[str, Any]) -> list[tuple[dict[str, Any], str]]:
    states: list[tuple[dict[str, Any], str]] = []
    state = event.get("state")
    if event.get("tool") == "task" and isinstance(state, dict):
        states.append((state, "/state"))
    part = event.get("part")
    if isinstance(part, dict) and part.get("tool") == "task" and isinstance(part.get("state"), dict):
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
            if key in state and _non_empty_result(state[key]):
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
        info = events[0].get("info")
        info_id = info.get("id") if isinstance(info, dict) else None
        if isinstance(info_id, str) and info_id:
            parent_id, parent_pointer = info_id, "/info/id"
        else:
            session_id = events[0].get("sessionID")
            parent_id, parent_pointer = (session_id, "/sessionID") if isinstance(session_id, str) and session_id else (None, None)
        if isinstance(parent_id, str) and parent_id:
            summary["parent"]["parent_id"] = parent_id
            summary["parent"]["evidence_offsets"] = [parent_pointer]
        else:
            limitations.add("parent_id_not_observed")
        candidates = []
        for message_index, message in enumerate(messages):
            if not isinstance(message, dict) or not isinstance(message.get("parts"), list):
                return _error_summary(source_file, format_name, "unsupported_export_structure")
            for part_index, part in enumerate(message["parts"]):
                if not isinstance(part, dict):
                    return _error_summary(source_file, format_name, "unsupported_export_structure")
                if part.get("tool") != "task" or not isinstance(part.get("state"), dict):
                    continue
                candidates.append((part["state"], f"/messages/{message_index}/parts/{part_index}/state", _json_pointer(["messages", message_index, "parts", part_index, "state"])))
        for state, base_pointer, evidence_pointer in candidates:
            receipt, state_limitations = _opencode_state_receipt(state, base_pointer, evidence_pointer)
            limitations.update(state_limitations)
            if receipt is None:
                continue
            receipts[receipt["child_id"]] = receipt
    else:
        for line_number, event in enumerate(events, start=1):
            session_id = event.get("sessionID")
            if isinstance(session_id, str) and session_id:
                if summary["parent"]["parent_id"] is None:
                    summary["parent"]["parent_id"] = session_id
                    summary["parent"]["evidence_offsets"].append(line_number)
                elif summary["parent"]["parent_id"] != session_id:
                    limitations.add("parent_id_conflict")
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
    if summary["parent"]["parent_id"] is None:
        limitations.add("parent_id_not_observed")
    return _finalize(summary, limitations)


def _parse_jsonl(text: str) -> tuple[list[dict[str, Any]] | None, str | None]:
    events: list[dict[str, Any]] = []
    if not text.strip():
        return None, "empty_input"
    saw_event = False
    for raw_line in text.splitlines():
        if not raw_line.strip():
            events.append({})
            continue
        saw_event = True
        try:
            value = json.loads(raw_line)
        except json.JSONDecodeError:
            return None, "truncated_or_invalid_jsonl"
        if not isinstance(value, dict):
            return None, "unsupported_jsonl_event"
        events.append(value)
    if not saw_event:
        return None, "empty_input"
    return events, None


def _looks_like_codex(events: list[dict[str, Any]]) -> bool:
    return any(
        event.get("type") in {"thread.started", "turn.started", "turn.running", "turn.completed", "turn.failed"}
        or (
            event.get("type") in CODEX_ITEM_TYPES
            and isinstance(event.get("item"), dict)
            and event["item"].get("type") == "collab_tool_call"
        )
        for event in events
    )


def _looks_like_opencode(events: list[dict[str, Any]]) -> bool:
    known_event_types = {"step_start", "step_finish", "tool_use", "tool_result", "message.updated"}
    return any(
        _state_from_event(event)
        or (
            event.get("type") in known_event_types
            and isinstance(event.get("sessionID"), str)
            and bool(event["sessionID"])
        )
        for event in events
    )


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
        summary = _opencode_summary(source_file, [document], requested_format, export=True)
        return summary, 0 if summary["parse_status"] == "ok" else 1

    if requested_format == "auto":
        try:
            document = json.loads(text)
        except json.JSONDecodeError:
            document = None
        if isinstance(document, dict) and isinstance(document.get("messages"), list):
            summary = _opencode_summary(source_file, [document], "opencode-export", export=True)
            return summary, 0 if summary["parse_status"] == "ok" else 1

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
    if requested_format == "codex-jsonl" and not _looks_like_codex(events):
        return _error_summary(source_file, requested_format, "unsupported_native_envelope"), 1
    if requested_format == "opencode-jsonl" and not _looks_like_opencode(events):
        return _error_summary(source_file, requested_format, "unsupported_native_envelope"), 1
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
