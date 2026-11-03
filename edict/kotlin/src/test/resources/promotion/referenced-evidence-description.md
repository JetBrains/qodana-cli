## Description

**Worker thread blocking**

Rule ID: `rule`

Reports blocking calls on the worker thread.

Estimated precision: **100.0%** · recall: **100.0%** · F1: **100.0%**

## Evidence

1. Fixed: "Avoid blocking the worker thread" in [PR #1](https://github.com/source/project/pull/1#discussion_r1).

2. Fixed: "positive 1" in [PR #2](https://github.com/source/project/pull/2#discussion_r2).

3. Fixed: "positive 2" in [PR #3](https://github.com/source/project/pull/3#discussion_r3).

4. Fixed: "positive 3" in [PR #4](https://github.com/source/project/pull/4#discussion_r4).

5. Fixed: "Avoid spinning until completion" in [d0d0caf](https://github.com/source/project/commit/d0d0cafed0d0cafed0d0cafed0d0cafed0d0cafe).

