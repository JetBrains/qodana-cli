# Edict Next MCP method audit

Source audited: Ultimate
`plugins/llm/qodana/agents/src/com/intellij/ml/llm/qodana/agents/edictnext/EdictNextMcpToolset.kt`
and every bundled Edict Next skill reference to its tools.

| Ultimate method | Skill use | Classification | Managed Kotlin decision |
| --- | --- | --- | --- |
| `edict_next_prepare_pipeline` | Preparation creates/loads a worktree and initializes one run session. | Pipeline/task management | Ignore. The trusted host supplies one managed state root; `edict-prepare` now freezes selected inbox bytes with `edict_validate_inbox`. |
| `edict_next_next_signal` | Distribution advances a server-owned alphabetical queue. | Pipeline/task management | Ignore. The managed distribution worker processes the receipt alphabetically. |
| `edict_next_get_distribution_context` | Distribution records that candidate context was read. | Pipeline/task management | Ignore. Scoped `edict_read` supplies complete persisted cluster context without a server cursor. |
| `edict_next_add_signal_to_cluster` | Distribution applies the only legacy distribution mutation. | Pipeline/task management/state mutation | Ignore. Managed hash-checked writes copy, verify, then delete; post-validation proves exact movement. |
| `edict_next_validate_distribution` | Root calls it after distribution and stops on failure. | State validation | Port as `edict_validate_distribution`, using the immutable receipt and server-side repository snapshot from `edict_validate_inbox`. It requires each selected Signal to move byte-for-byte into exactly one cluster, rejects unrelated state changes, and validates repository invariants. |
| `edict_next_get_generation_clusters` | Generation freezes Pending targets and supplies concurrency. | Pipeline/task management | Ignore. The managed coordinator receives explicit affected/Pending IDs and controls native worker capacity. |
| `edict_next_validate_code_example` | Every code-example worker calls it before assigning the example. | State validation plus IDE parsing | Split. `edict_validate_code_example` ports persisted metadata/layout/language/label/range checks; the skill performs the PSI/parser check with Ultimate's generic inspection MCP before assignment. |
| `edict_next_validate_cluster_examples` | Overseer, cluster generation, and weak review require it after reconciliation. | State validation plus IDE parsing | Split. `edict_validate_cluster_examples` ports complete Signal/example linkage and structural state checks; skills retain Ultimate PSI/parser checks per example. |
| `edict_next_get_inspection_action` | Cluster generation detects conflicts and executes a predecessor against examples to choose reuse or generation. | Generated-inspection execution plus session deadline/action state | Do not port. Managed generation reviews and measures the previous version with the same generic `run_inspection_kts` calls as a new candidate. |
| `edict_next_validate_inspection` | Cluster generation compiles the candidate and measures all examples. | Generated-inspection execution | Keep in Ultimate. Managed skills pass exact `edict_read` candidate bytes as `inspectionKtsCode`, the inspected source project as `projectPath`, and enforce strong-example/85% accuracy gates. |
| `edict_next_get_new_inspection_results` | Cluster generation executes the candidate over the inspected project and builds weak-review inputs. | Generated-inspection execution | Keep in Ultimate. Managed skills use the same generic `run_inspection_kts` call shape and store complete findings/manifests only in private scratch. |
| `edict_next_mark_generated` | Applies a guarded legacy session transition after action, analysis, and validation receipts. | Session/task management plus state mutation | Ignore. Managed hash-checked writes publish exact accepted bytes; state validation is separate from external execution/review evidence. |
| `edict_next_validate_generation` | Root validates frozen state, then recompiles every Generated inspection before publication. | Mixed state validation and generated-inspection execution | Split. `edict_validate_generation` ports repository/target state checks. Skills must first verify Ultimate compilation, example/project runs, reviews, and exact-candidate hashes; the managed validator explicitly does not replace them. |

The resulting boundary is intentional: managed Kotlin owns authoritative state and read-only state validation; Ultimate
owns PSI parsing and all generated-inspection compilation/execution; native agents own orchestration and task lifecycle.
