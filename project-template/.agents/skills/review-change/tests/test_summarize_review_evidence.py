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

    def test_codex_completed_reviewer_requires_spawn_and_result_location(self):
        completed, summary = self.run_helper(
            "\n".join(
                [
                    json.dumps({"type": "collab.spawn", "child_id": "child-1", "role": "td_reviewer"}),
                    json.dumps(
                        {
                            "type": "collab.wait",
                            "child_id": "child-1",
                            "role": "td_reviewer",
                            "status": "completed",
                            "result_location": "artifacts/review.json",
                        }
                    ),
                    json.dumps({"type": "turn.completed"}),
                ]
            )
            + "\n",
        )
        self.assertEqual(completed.returncode, 0)
        self.assertEqual(summary["parse_status"], "ok")
        self.assertEqual(summary["format"], "codex-jsonl")
        self.assertEqual(summary["outer_status"], "completed")
        self.assertEqual(summary["data_sufficiency"], "sufficient")
        self.assertEqual(
            summary["review_receipts"],
            [
                {
                    "child_id": "child-1",
                    "evidence_offsets": [1, 2],
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

    def test_codex_missing_child_role_and_empty_wait_are_not_receipts(self):
        _, summary = self.run_helper(
            "\n".join(
                [
                    json.dumps({"type": "collab.spawn", "role": "td_reviewer"}),
                    json.dumps({"type": "collab.wait", "status": "completed", "role": "td_reviewer"}),
                    json.dumps({"type": "collab.spawn", "child_id": "child-2"}),
                    json.dumps({"type": "collab.wait", "child_id": "child-2", "status": "completed"}),
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
                    json.dumps({"type": "collab.spawn", "child_id": "failed", "role": "td_reviewer"}),
                    json.dumps({"type": "collab.wait", "child_id": "failed", "role": "td_reviewer", "status": "failed"}),
                    json.dumps({"type": "collab.spawn", "child_id": "running", "role": "td_reviewer"}),
                    json.dumps({"type": "collab.wait", "child_id": "running", "role": "td_reviewer", "status": "running"}),
                ]
            )
            + "\n"
        )
        self.assertEqual(summary["data_sufficiency"], "partial")
        self.assertEqual([receipt["lifecycle_status"] for receipt in summary["review_receipts"]], ["failed", "running"])
        self.assertTrue(all(receipt["result_location"] is None for receipt in summary["review_receipts"]))

    def test_non_review_role_and_self_claim_are_not_review_receipts(self):
        _, summary = self.run_helper(
            "\n".join(
                [
                    json.dumps({"type": "collab.spawn", "child_id": "builder", "role": "td_implementer"}),
                    json.dumps({"type": "collab.wait", "child_id": "builder", "role": "td_implementer", "status": "completed", "result_location": "x"}),
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
        receipt = summary["review_receipts"][0]
        self.assertEqual(receipt["child_id"], "ses-1")
        self.assertEqual(receipt["result_location"], "/state/output")
        self.assertEqual(receipt["evidence_offsets"], [1])
        self.assertNotIn("private review text", completed.stdout)

    def test_opencode_export_only_traverses_messages_and_parts(self):
        completed, summary = self.run_helper(
            json.dumps(
                {
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
        self.assertEqual(summary["review_receipts"][0]["child_id"], "ses-export")
        self.assertEqual(summary["review_receipts"][0]["result_location"], "/messages/0/parts/0/state/result")
        self.assertNotIn("review.json", completed.stdout)

    def test_invalid_or_unsupported_input_is_error_and_unknown(self):
        completed, summary = self.run_helper("{\"type\": \"turn.started\"}\n{\"broken\"\n")
        self.assertNotEqual(completed.returncode, 0)
        self.assertEqual(summary["parse_status"], "error")
        self.assertEqual(summary["data_sufficiency"], "unknown")

        completed, summary = self.run_helper("{\"hello\": \"world\"}\n")
        self.assertNotEqual(completed.returncode, 0)
        self.assertEqual(summary["parse_status"], "error")
        self.assertEqual(summary["data_sufficiency"], "unknown")


if __name__ == "__main__":
    unittest.main()
