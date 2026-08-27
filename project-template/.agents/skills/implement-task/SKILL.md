---
name: implement-task
description: Use when implementing one approved Task within its assigned repository paths.
---

# Implement task

1. Load the approved contract and select one Task. Confirm its allowed paths, protected paths, acceptance criteria, dependencies, and verification. Completion: every intended edit belongs to that Task.
2. Inspect the assigned Worktree for unrelated or dirty changes and preserve them. Implement the smallest in-scope solution, with a failing focused test before production behavior changes. Completion: no edit crosses the Task boundary.
3. Run the Task verification and required repository checks. Commit the verified change with Korean context. Completion: the commit SHA is available.
4. Return this result schema:

   ```text
   task_id: <Task ID>
   status: complete | blocked
   changed_paths: <paths or none>
   checks: <command: result>
   commit_sha: <SHA or none>
   remaining_risk: <risk or none>
   blocker: <reason or none>
   ```

   If no in-scope solution exists, stop without widening scope and return `status: blocked` with the reason and `commit_sha: none`. Completion: all seven fields are present.
