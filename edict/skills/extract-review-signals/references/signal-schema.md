# Signal JSON schema (`InspectionSpecification`)

Each file in `.qodana/edict/inbox_signals/` is one signal — a JSON object matching the
`InspectionSpecification` model of the Qodana edict pipeline (kotlinx.serialization format).
Field names, nesting, and the `source.type` discriminator must match exactly.

## Top-level object

| Field | Type | Required | Meaning |
|---|---|---|---|
| `ruleId` | string | yes | Unique kebab-case identifier, e.g. `avoid-blocking-io-in-coroutines` |
| `ruleName` | string | yes | Short, actionable title |
| `description` | string | yes | The problem, possible solutions, and impact — written for a developer who sees the warning |
| `severity` | string | yes | One of `ERROR`, `WARNING`, `WEAK_WARNING`, `INFO` |
| `language` | string | yes | One of `Kotlin`, `Java`, `Unknown` (exact spelling) |
| `positiveExamples` | FileRevision[] | yes | Revisions where the violation IS present (comment state) |
| `negativeExamples` | FileRevision[] | yes | Revisions where the violation is absent/fixed (head state) |
| `optionalPositiveExamples` | FileRevision[] | no (default `[]`) | Extra violation examples, e.g. merged from a duplicate thread |
| `optionalNegativeExamples` | FileRevision[] | no (default `[]`) | Extra fixed examples |
| `source` | RuleExtractionSource | yes | Provenance; for review signals always the `FromPR` variant |

## `FileRevision`

| Field | Type | Required | Meaning |
|---|---|---|---|
| `path` | string | yes | Repo-relative file path at that revision (head-state path for negative examples if the file was renamed) |
| `revision` | string | yes | Commit SHA; must not be blank |
| `expectedProblemRanges` | LineRange[] or null | yes (nullable) | 1-based inclusive line ranges of the violation at that revision; `null` when the location cannot be determined (e.g. code deleted in the fix) |

## `LineRange`

```json
{"start": 42, "end": 44}
```

1-based, inclusive on both ends. Each range must tightly bound the violating expression — not the
enclosing method or class.

## `source` — `FromPR` variant

| Field | Type | Meaning |
|---|---|---|
| `type` | string | Discriminator, literally `"FromPR"` |
| `prNumber` | int | PR/MR number |
| `title` | string | PR title |
| `url` | string | PR web URL |
| `reviewDiscussionUrl` | string | Deep link to the review thread (GitHub: `<prUrl>#discussion_r<id>`; Space: `.../review/<number>`) |
| `threadId` | string | Stable id: `<provider>-<prNumber>-<rootCommentId>`, e.g. `github-123-987654` |
| `originalCommentAuthor` | string | Author of the root review comment |
| `possibleProblemRegions` | LineRange[] | Problem regions in the comment state (same values as `positiveExamples[0].expectedProblemRanges`) |
| `discussionMessages` | string[] | All thread messages as `"author (timestamp): body"` strings, in order |
| `diffBaseToComment` | string | Unified diff of the file between `baseSha` and `originalCommitSha` (~3 context lines) |
| `diffCommentToHead` | string | Unified diff of the file between `originalCommitSha` and `headSha` (~3 context lines) |
| `verificationDate` | long | PR merge/close time, epoch **milliseconds** |

## Complete example

```json
{
  "ruleId": "close-stream-in-finally",
  "ruleName": "Streams must be closed in finally or use-blocks",
  "description": "InputStream opened here is not closed when an exception is thrown between open and close. Wrap the stream in a use { } block (Kotlin) or try-with-resources (Java) so it is always released.",
  "severity": "WARNING",
  "language": "Kotlin",
  "positiveExamples": [
    {
      "path": "src/main/kotlin/com/example/io/ConfigLoader.kt",
      "revision": "8c1f3a9d2e5b47c6a0d1f2e3b4a5c6d7e8f90123",
      "expectedProblemRanges": [{"start": 57, "end": 59}]
    }
  ],
  "negativeExamples": [
    {
      "path": "src/main/kotlin/com/example/io/ConfigLoader.kt",
      "revision": "d4e5f60718293a4b5c6d7e8f9012345678abcdef",
      "expectedProblemRanges": [{"start": 57, "end": 60}]
    }
  ],
  "optionalPositiveExamples": [],
  "optionalNegativeExamples": [],
  "source": {
    "type": "FromPR",
    "prNumber": 482,
    "title": "Add config hot-reload",
    "url": "https://github.com/acme/service/pull/482",
    "reviewDiscussionUrl": "https://github.com/acme/service/pull/482#discussion_r199887766",
    "threadId": "github-482-199887766",
    "originalCommentAuthor": "jsmith",
    "possibleProblemRegions": [{"start": 57, "end": 59}],
    "discussionMessages": [
      "jsmith (2026-05-11T09:14:02Z): This stream leaks if parse() throws — please close it in a use block.",
      "author (2026-05-11T10:02:41Z): Good catch, fixed."
    ],
    "diffBaseToComment": "@@ -50,6 +55,8 @@\n         val cfg = path.toFile()\n+        val stream = cfg.inputStream()\n+        return parse(stream)\n",
    "diffCommentToHead": "@@ -55,4 +55,5 @@\n-        val stream = cfg.inputStream()\n-        return parse(stream)\n+        return cfg.inputStream().use { stream ->\n+            parse(stream)\n+        }\n",
    "verificationDate": 1778234561000
  }
}
```

## Extraction-decision object (internal, not written to the inbox)

While deciding, structure the intermediate result as one of:

```json
{"result": {"kind": "ExtractedRule", "ruleId": "...", "ruleName": "...", "description": "...",
            "reason": "Why pattern-based checkable + which comment",
            "severity": "WARNING",
            "problemRegions": [{"problemStartLine": 57, "problemEndLine": 59}]}}
```

```json
{"result": {"kind": "NoRule", "reason": "Brief explanation of why no rule could be extracted"}}
```

`problemRegions` use line numbers from the comment-state file slice. The `reason` field is for the
trace only; the inbox signal does not include it.
