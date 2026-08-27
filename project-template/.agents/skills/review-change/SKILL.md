---
name: review-change
description: Use when independently reviewing one Task change before it can proceed through the merge gate.
---

# Review change

1. Review from a context separate from the implementation session. Load the approved Task, diff, changed-path ownership, check evidence, and current shared Repair Round count. Completion: the evidence can be evaluated without relying on the Builder’s conclusion.
2. Test the acceptance criteria, scope boundary, regressions, protected changes, and whether CI was weakened or bypassed. Completion: each finding has a path or command-evidence reference.
3. Return this result schema, with blocking findings before recommendations:

   ```text
   status: approved | blocked
   repair_round: <0|1|2>/2
   blocking_findings: <findings or none>
   recommendations: <ordered items or none>
   evidence: <paths and check results>
   ```

   A blocking result consumes the shared Repair Round budget. At `2/2`, return `status: blocked` and recommend operator review instead of another automatic repair. Completion: the result contains all five fields.
