import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "summarize_review_evidence.py"


class SummarizeReviewEvidenceTests(unittest.TestCase):
    def run_helper(self, content, *extra_args):
        with tempfile.TemporaryDirectory() as directory:
            artifact = Path(directory) / "trace.jsonl"
            artifact.write_text(content, encoding="utf-8")
            completed = subprocess.run(
                [sys.executable, str(SCRIPT), *extra_args, str(artifact)],
                capture_output=True,
                text=True,
            )
            self.assertTrue(completed.stdout, completed.stderr)
            return completed, json.loads(completed.stdout)

    @staticmethod
    def codex_item(event_type, tool, **fields):
        return {"type": event_type, "item": {"type": "collab_tool_call", "tool": tool, **fields}}

    def test_codex_completed_reviewer_requires_spawn_and_result_location(self):
        completed, summary = self.run_helper(
            "\n".join(
                [
                    json.dumps({"type": "thread.started", "thread_id": "parent-1"}),
                    json.dumps(self.codex_item("item.started", "spawn_agent", agent_type="td_reviewer", receiver_thread_ids=["child-1"])),
                    json.dumps(self.codex_item("item.completed", "wait_agent", agent_type="td_reviewer", receiver_thread_ids=["child-1"], status="completed", result_location="artifacts/review.json")),
                    json.dumps({"type": "turn.completed"}),
                ]
            )
            + "\n",
        )
        self.assertEqual(completed.returncode, 0)
        self.assertEqual(summary["parse_status"], "ok")
        self.assertEqual(summary["format"], "codex-jsonl")
        self.assertEqual(summary["outer_status"], "completed")
        self.assertEqual(summary["parent"], {"parent_id": "parent-1", "lifecycle_status": "unknown", "evidence_offsets": [1]})
        self.assertEqual(summary["data_sufficiency"], "sufficient")
        self.assertEqual(
            summary["review_receipts"],
            [
                {
                    "child_id": "child-1",
                    "evidence_offsets": [2, 3],
                    "explicit_role": "td_reviewer",
                    "lifecycle_status": "completed",
                    "result_location": "artifacts/review.json",
                }
            ],
        )
        self.assertEqual(completed.stdout, json.dumps(summary, sort_keys=True, separators=(",", ":")) + "\n")

    def test_codex_turn_completion_does_not_promote_without_serialized_spawn(self):
        _, summary = self.run_helper(
            json.dumps(
                {
                    "type": "turn.completed",
                    "child_id": "child-1",
                    "role": "td_reviewer",
                    "result_location": "review.json",
                }
            )
            + "\n"
        )
        self.assertEqual(summary["review_receipts"], [])
        self.assertEqual(summary["data_sufficiency"], "partial")

    def test_jsonl_offsets_retain_original_line_numbers_across_blank_lines(self):
        _, summary = self.run_helper(
            "\n".join(
                [
                    "",
                    json.dumps({"type": "thread.started", "thread_id": "parent-1"}),
                    json.dumps(self.codex_item("item.started", "spawn_agent", agent_type="td_reviewer", receiver_thread_ids=["child-1"])),
                    json.dumps(self.codex_item("item.completed", "wait_agent", agent_type="td_reviewer", receiver_thread_ids=["child-1"], status="completed", result_location="review.json")),
                ]
            )
            + "\n"
        )
        self.assertEqual(summary["review_receipts"][0]["evidence_offsets"], [3, 4])

    def test_codex_missing_child_role_and_empty_wait_are_not_receipts(self):
        _, summary = self.run_helper(
            "\n".join(
                [
                    json.dumps({"type": "thread.started", "thread_id": "parent-1"}),
                    json.dumps(self.codex_item("item.started", "spawn_agent", agent_type="td_reviewer")),
                    json.dumps(self.codex_item("item.completed", "wait_agent", agent_type="td_reviewer", status="completed")),
                    json.dumps(self.codex_item("item.started", "spawn_agent", receiver_thread_ids=["child-2"])),
                    json.dumps(self.codex_item("item.completed", "wait_agent", receiver_thread_ids=["child-2"], status="completed")),
                ]
            )
            + "\n"
        )
        self.assertEqual(summary["review_receipts"], [])
        self.assertEqual(summary["data_sufficiency"], "partial")

    def test_codex_failed_and_running_receipts_are_partial(self):
        _, summary = self.run_helper(
            "\n".join(
                [
                    json.dumps({"type": "thread.started", "thread_id": "parent-1"}),
                    json.dumps(self.codex_item("item.started", "spawn_agent", agent_type="td_reviewer", receiver_thread_ids=["failed"])),
                    json.dumps(self.codex_item("item.completed", "wait_agent", agent_type="td_reviewer", receiver_thread_ids=["failed"], status="failed")),
                    json.dumps(self.codex_item("item.started", "spawn_agent", agent_type="td_reviewer", receiver_thread_ids=["running"])),
                    json.dumps(self.codex_item("item.completed", "wait_agent", agent_type="td_reviewer", receiver_thread_ids=["running"], status="running")),
                ]
            )
            + "\n"
        )
        self.assertEqual(summary["data_sufficiency"], "partial")
        self.assertEqual([receipt["lifecycle_status"] for receipt in summary["review_receipts"]], ["failed", "running"])
        self.assertTrue(all(receipt["result_location"] is None for receipt in summary["review_receipts"]))

    def test_codex_spawn_completion_does_not_promote_and_wait_alias_empty_is_partial(self):
        _, summary = self.run_helper(
            "\n".join(
                [
                    json.dumps({"type": "thread.started", "thread_id": "parent-1"}),
                    json.dumps(self.codex_item("item.completed", "spawn_agent", agent_type="td_reviewer", receiver_thread_ids=["child"], status="completed")),
                    json.dumps(self.codex_item("item.completed", "wait", agent_type="td_reviewer", status="completed", result_location="review.json")),
                ]
            )
            + "\n"
        )
        self.assertEqual(summary["data_sufficiency"], "partial")
        receipt = summary["review_receipts"][0]
        self.assertEqual(receipt["lifecycle_status"], "started")
        self.assertIsNone(receipt["result_location"])

    def test_non_review_role_and_self_claim_are_not_review_receipts(self):
        _, summary = self.run_helper(
            "\n".join(
                [
                    json.dumps({"type": "thread.started", "thread_id": "parent-1"}),
                    json.dumps(self.codex_item("item.started", "spawn_agent", agent_type="td_implementer", receiver_thread_ids=["builder"])),
                    json.dumps(self.codex_item("item.completed", "wait_agent", agent_type="td_implementer", receiver_thread_ids=["builder"], status="completed", result_location="x")),
                    json.dumps({"type": "turn.completed", "role": "td_reviewer", "result_location": "self.txt"}),
                ]
            )
            + "\n"
        )
        self.assertEqual(summary["review_receipts"], [])
        self.assertEqual(summary["data_sufficiency"], "partial")

    def test_opencode_completed_task_uses_json_pointer_for_result_field(self):
        completed, summary = self.run_helper(
            json.dumps(
                {
                    "type": "task",
                    "tool": "task",
                    "sessionID": "parent-opencode",
                    "state": {
                        "input": {"subagent_type": "td_reviewer"},
                        "metadata": {"sessionId": "ses-1"},
                        "status": "completed",
                        "output": "private review text",
                    },
                }
            )
            + "\n",
            "--format",
            "opencode-jsonl",
        )
        self.assertEqual(completed.returncode, 0)
        self.assertEqual(summary["data_sufficiency"], "sufficient")
        self.assertEqual(summary["parent"], {"parent_id": "parent-opencode", "lifecycle_status": "unknown", "evidence_offsets": [1]})
        receipt = summary["review_receipts"][0]
        self.assertEqual(receipt["child_id"], "ses-1")
        self.assertEqual(receipt["result_location"], "/state/output")
        self.assertEqual(receipt["evidence_offsets"], [1])
        self.assertNotIn("private review text", completed.stdout)

    def test_opencode_export_only_traverses_messages_and_parts(self):
        completed, summary = self.run_helper(
            json.dumps(
                {
                    "info": {"id": "parent-export"},
                    "metadata": {"state": {"input": {"subagent_type": "td_reviewer"}}},
                    "messages": [
                        {
                            "parts": [
                                {
                                    "type": "tool",
                                    "tool": "task",
                                    "state": {
                                        "input": {"subagent_type": "td_reviewer"},
                                        "metadata": {"sessionId": "ses-export"},
                                        "status": "completed",
                                        "result": {"path": "review.json"},
                                    },
                                }
                            ]
                        }
                    ],
                }
            ),
            "--format",
            "opencode-export",
        )
        self.assertEqual(completed.returncode, 0)
        self.assertEqual(summary["data_sufficiency"], "sufficient")
        self.assertEqual(summary["parent"]["parent_id"], "parent-export")
        self.assertEqual(summary["parent"]["evidence_offsets"], ["/info/id"])
        self.assertEqual(summary["review_receipts"][0]["child_id"], "ses-export")
        self.assertEqual(summary["review_receipts"][0]["result_location"], "/messages/0/parts/0/state/result")
        self.assertNotIn("review.json", completed.stdout)

        _, summary = self.run_helper(
            json.dumps({"info": {"id": 123}, "sessionID": "fallback-parent", "messages": []}),
            "--format",
            "opencode-export",
        )
        self.assertEqual(summary["parent"]["parent_id"], "fallback-parent")
        self.assertEqual(summary["parent"]["evidence_offsets"], ["/sessionID"])

    def test_opencode_non_task_state_and_nested_prompt_are_not_receipts(self):
        completed, summary = self.run_helper(
            json.dumps(
                {
                    "type": "tool_use",
                    "tool": "shell",
                    "sessionID": "parent-opencode",
                    "state": {
                        "input": {"subagent_type": "td_reviewer"},
                        "metadata": {"sessionId": "fake"},
                        "status": "completed",
                        "output": "task tool text in a shell result",
                    },
                    "prompt": "{\"tool\":\"task\",\"state\":{\"input\":{\"subagent_type\":\"td_reviewer\"}}}",
                }
            ),
            "--format",
            "opencode-jsonl",
        )
        self.assertEqual(summary["review_receipts"], [])
        self.assertEqual(summary["data_sufficiency"], "partial")
        self.assertEqual(completed.returncode, 0)

    def test_opencode_normal_no_review_events_are_supported_partial(self):
        content = "\n".join(
            [
                json.dumps({"type": "step_start", "sessionID": "parent-opencode"}),
                json.dumps({"type": "tool_use", "tool": "read", "sessionID": "parent-opencode"}),
                json.dumps({"type": "step_finish", "sessionID": "parent-opencode"}),
            ]
        ) + "\n"
        completed, summary = self.run_helper(content)
        self.assertEqual(completed.returncode, 0)
        self.assertEqual(summary["format"], "opencode-jsonl")
        self.assertEqual(summary["data_sufficiency"], "partial")
        self.assertEqual(summary["review_receipts"], [])

    def test_opencode_export_uses_latest_state_and_requires_nonempty_result(self):
        messages = []
        for index in range(11):
            if index == 2:
                state = {
                    "input": {"subagent_type": "td_reviewer"},
                    "metadata": {"sessionId": "same-child"},
                    "status": "completed",
                    "result": {"review": "recorded"},
                }
            elif index == 10:
                state = {
                    "input": {"subagent_type": "td_reviewer"},
                    "metadata": {"sessionId": "same-child"},
                    "status": "failed",
                }
            else:
                state = {"input": {"subagent_type": "td_implementer"}, "metadata": {"sessionId": f"other-{index}"}, "status": "completed", "result": {"ok": True}}
            messages.append({"parts": [{"type": "tool", "tool": "task", "state": state}]})
        _, summary = self.run_helper(json.dumps({"info": {"id": "parent-export"}, "messages": messages}), "--format", "opencode-export")
        self.assertEqual(summary["data_sufficiency"], "partial")
        self.assertEqual(summary["review_receipts"][0]["lifecycle_status"], "failed")

        for empty_value in (None, "", {}, []):
            _, summary = self.run_helper(
                json.dumps(
                    {
                        "info": {"id": "parent-export"},
                        "messages": [{"parts": [{"tool": "task", "state": {"input": {"subagent_type": "td_reviewer"}, "metadata": {"sessionId": "child"}, "status": "completed", "result": empty_value}}]}],
                    }
                ),
                "--format",
                "opencode-export",
            )
            self.assertEqual(summary["data_sufficiency"], "partial")
            self.assertEqual(summary["review_receipts"][0]["lifecycle_status"], "unknown")

    def test_opencode_export_invalid_part_is_error_without_traceback(self):
        completed, summary = self.run_helper(
            json.dumps({"info": {"id": "parent-export"}, "messages": [{"parts": [None]}]}),
            "--format",
            "opencode-export",
        )
        self.assertNotEqual(completed.returncode, 0)
        self.assertEqual(summary["parse_status"], "error")
        self.assertEqual(summary["data_sufficiency"], "unknown")

        _, summary = self.run_helper(
            json.dumps(
                {
                    "sessionID": "parent-export",
                    "messages": [
                        {
                            "parts": [
                                {
                                    "type": "tool",
                                    "tool": "shell",
                                    "state": {
                                        "input": {"subagent_type": "td_reviewer"},
                                        "metadata": {"sessionId": "fake"},
                                        "status": "completed",
                                        "output": "not a task result",
                                    },
                                }
                            ]
                        }
                    ],
                }
            ),
            "--format",
            "opencode-export",
        )
        self.assertEqual(summary["review_receipts"], [])
        self.assertEqual(summary["data_sufficiency"], "partial")

    def test_invalid_or_unsupported_input_is_error_and_unknown(self):
        completed, summary = self.run_helper("{\"type\": \"turn.started\"}\n{\"broken\"\n")
        self.assertNotEqual(completed.returncode, 0)
        self.assertEqual(summary["parse_status"], "error")
        self.assertEqual(summary["data_sufficiency"], "unknown")

        completed, summary = self.run_helper("{\"hello\": \"world\"}\n")
        self.assertNotEqual(completed.returncode, 0)
        self.assertEqual(summary["parse_status"], "error")
        self.assertEqual(summary["data_sufficiency"], "unknown")

        for format_name in ("codex-jsonl", "opencode-jsonl"):
            completed, summary = self.run_helper("{\"hello\": \"world\"}\n", "--format", format_name)
            self.assertNotEqual(completed.returncode, 0)
            self.assertEqual(summary["parse_status"], "error")
            self.assertEqual(summary["data_sufficiency"], "unknown")


if __name__ == "__main__":
    unittest.main()
