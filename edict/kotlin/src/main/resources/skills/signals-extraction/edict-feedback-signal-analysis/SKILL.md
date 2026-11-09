---
name: edict-feedback-signal-analysis
description: Managed subagent that turns users' 👍/👎 answers to Edict's review comments on pull requests into inbox Signals.
---

# Feedback Signal Analysis

Run only as a delegated managed subagent. Follow [the managed task protocol](../edict_manager/references/task-protocol.md)
and [the signal contract](../edict_manager/references/signals.md). Registry ID: `edict-feedback-signal-analysis`.
You own `inbox.write` for `SubmittedFeedback` Signals of feedback records. Do not change source, clusters, examples,
inspections, or records by hand.

Edict comments on pull requests with findings of inspections that are not promoted yet, one feedback record per
comment, and asks users to react with 👍 when the finding is a real problem or with 👎 when it is not. The users'
thumbs decide each Signal's label, and the Signal keeps the replies as their authors wrote them; you only write the
description the record lacks.

Start the task.

1. Call `edict_collect_feedback_answers` once. It returns the answered records, each with the label its thumbs give, the
   record (inspection, problem message, path, revision and lines), and the comment's URL and replies. It moves
   unanswered records of closed pull requests to history itself; never retry it to find more answers.
2. For each answer, read the record's lines at its revision when the project checkout has that commit, and the
   replies. Write its `description`: one line, at most 1000 characters, saying what the lines demonstrate under the
   label: for POSITIVE, the problem the code has; for NEGATIVE, why the code is fine although the inspection reported
   it. Do not name the inspection, its id, a severity, or a fix.
3. Call `edict_publish_feedback_signal` with the record id and description. The server takes the label from the
   comment's thumbs, builds the Signal and moves the record to history. Report a refused record and continue.

Complete the task with the number of published Signals per label and the record ids that failed, with their errors.
