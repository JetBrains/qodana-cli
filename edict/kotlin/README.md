# Edict

Edict is a set of tools and agent's skills to extract custom static rules based on your project's development history.
Right now Edict supports only Qodana static analysis engine for Kotlin and Java. Edict comes with the Qodana CLI as
`qodana edict`.

To build or change Edict, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Quick start

Run all commands from the project directory:

```sh
qodana edict install                 # install skills and .codex/config.toml, check out the state branch
qodana edict mcp start               # start the Edict server; returns once it is ready
codex exec '$edict_manager gather new signals and generate new rules'
qodana edict mcp stop
qodana edict state push              # commit the state and push its branch to origin
```

## Edict manager

Edict manager is an entry gate skill to any available actions in Edict.
Edict manager has a default pipeline adjusted to work as a daily routine for incremental signals gathering and rule adjustments.  

Default pipeline includes:
1. **Signal extraction.** Find code corrections in commits and code reviews, and store them as Signals in the inbox
   of Edict state. Edict state is a repo with gathered data and candidate rules.
2. **Signal distribution.** Group all inbox Signal into clusters of similar Signals.
3. **Rule generation.** Write a rule for selected cluster. Then measure its precision and recall against
   the Signals and against a full project analysis. This step includes several iterations with verification of rule
   quality. Including review, evaluating of results for project wide analysis, retrospective analysis.
4. **Rule promotion.** Open a pull request in a project repo with enforcement of the rule if there is enough evidence of rule
   quality. If rule does not have enough evidence it stays in experimental mode and will be used for analysis but in
   limited scope and special disclaimer.

After promotion rule becomes a part of your project's default profile and will be used for analysis wherever you did set it up: IDE, CI/CD quality gates, agent's development pipelines. Edict manager has a certain capabilities to adjust or split pipeline through correcting a task/propmt provided to edict manager skill.
Edict manager requires available Edict mcp server. All results of work are stored in Edict state manipulated by MCP server.

## Requirements

- Git. The project must be a Git repository.
- [Codex](https://github.com/openai/codex), installed and on `PATH` (or set `$CODEX_BIN`).
- A Codex configuration in `$CODEX_HOME/config.toml` that has your model provider and marks the project as trusted:

  ```toml
  [projects."/path/to/project"]
  trust_level = "trusted"
  ```

  Codex ignores the project's `.codex/config.toml` unless the project is trusted. Edict never writes to `$CODEX_HOME`.
- For review extraction and promotion: a GitHub token (see [Repositories](#repositories)).

Start every Codex request with `$edict_manager`, for example:

- `$edict_manager extract signals from the latest commit`
- `$edict_manager process all reviews previous weak and generate new rules`

## Commands

| Command                                                                 | Description                                                                                                                                    |
|-------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `qodana edict install [--deny <path>]... [--config <file>]`             | Installs the managed skills into `.codex/skills` and writes `.codex/config.toml`. Checks out the state branch as a worktree at the state path. |
| `qodana edict mcp start [--foreground] [--ide-* ...] [--config <file>]` | Starts the Edict MCP server on loopback port `edict.mcpPort`. Runs in the background unless you pass `--foreground`.                           |
| `qodana edict mcp stop`                                                 | Stops the server started for the current directory.                                                                        |
| `qodana edict state push [--message <msg>] [--config <file>]`           | Commits the state and pushes the state branch to origin.                                                                                       |

`--config` defaults to `qodana.yml` or `qodana.yaml` in the current directory.

### `install`

`install` writes these files and folders to the project:

- `.codex/skills/`: edict skills. Other skills already in the folder are kept.
- `.codex/config.toml`: the `edict` permission profile, agent limits, and the `edict-mcp` server URL.
    - Agents can read the project and write only to `log/agent-work`.
    - Agents can't access `log/process-log`, `$CODEX_HOME/sessions`, `$CODEX_HOME/log`, or any path you pass with
      `--deny`.
- The state branch `edict.stateBranch`, checked out as a worktree at `edict.statePath`:
    - It is taken from origin when origin has it, and created otherwise.
    - It shares no history with the project's branches, and the project's checkout stays on its current branch.
    - If the state path already contains files, they are used as they are.

`install` then runs `codex mcp list --json` to check that Codex loads the configuration. If Codex doesn't load it,
`install` removes the configuration and fails. `install` also fails outside a Git repository.

Run `install` again after you change `edict.mcpPort`.

### `mcp start`

The server is shared by every agent of a run. It fails right away if `.codex/config.toml` is missing, names a
different port, or the port is busy. It never falls back to another port.

### `state push`

`state push` works like this:

- It commits every change in the state directory with the message from `--message` (default
  `Update the Edict state`).
- If origin already has a newer version of the branch, it rebases the commit onto it, then pushes the branch with
  upstream tracking.
- It fails while the server is running, so run `qodana edict mcp stop` first.
- It also fails when the state path isn't a worktree on `edict.stateBranch`, for example when `install` kept an
  existing plain directory there.

## Edict state
TODO

## Configuration

Edict reads the `edict` section of `qodana.yml` (or `qodana.yaml`). Every key is optional, and the values below are
the defaults:

```yaml
edict:
  mcpPort: 27182
  statePath: .edict                  # relative to the project
  stateBranch: edict/state
  projectAnalysisTimeoutMinutes: 30  # per whole-project inspection analysis
  extraction:
    defaultSignalCount: 100          # new Signals per extraction run
  distribution:
    defaultSignalCount: 300          # inbox Signals distributed per run
  generation:
    maxProjectAnalyses: 3            # project analyses per cluster in one run
    defaultGenerationCount: 5        # clusters generated per run
  calculatePrice: false              # report model cost per run
  ci:
    url: https://github.com/owner/repo            # where code reviews are read
  promotion:
    url: https://github.com/owner/repo            # where promotion PRs are opened; defaults to ci.url
    reviewer: reviewer-login                      # required to enable promotion
    targetBranch: main                            # defaults to the repository's default branch
    inspectionsDirectory: inspections
    targetPrecision: 0.7
    targetRecall: 0.6
    targetF1: 0.0
    minSourceFiles: 5                             # distinct source files with strong positive Signals
  feedback:
    maxPerInspection: 10                          # unanswered comments of one inspection at once
    maxPerPullRequest: 3                          # review comments per pull request over all runs
  bots: # accounts whose review comments are ignored
    - KotlinBuild
```

### Repositories

Repository URLs have the form `https://<host>/<owner>/<repo>`. This covers github.com, GitHub Enterprise Cloud, and
GitHub Enterprise Server.

Set `GITHUB_TOKEN` or `GH_TOKEN` in the environment of `qodana edict mcp start`. The server doesn't start without a
token when a repository is configured.

Edict ignores review comments from bots. GitHub already flags its own apps. Add accounts that post as
regular users, such as a build server, to `edict.bots`.

Changing `promotion.url` while promotion PRs are still in review is not supported.

### Promotion

Promotion proposes generated inspections as pull requests. An inspection is a candidate if it meets two conditions:

- Its precision, recall, and F1 are at least the `target*` values.
- It has strong positive Signals from at least `minSourceFiles` distinct source files.

The promotion skill reviews the candidates and decides which to propose.

When `QODANA_TOKEN` is set, each newly generated inspection uploads its project analysis to Qodana Cloud. The promotion
PR then links to that report.

### Feedback

`state push` also writes `feedback/inspections.json`. It lists the generated inspections, in any cluster state, that
meet all of these conditions:

- Their evaluation matches their bytes and current examples, with at least `targetPrecision`.
- No promotion proposed them.
- Their findings got at most five false-positive answers that no generation took into account yet.

`qodana edict feedback prepare` and `qodana edict feedback post` read only that file. A pull request gets at most
`maxPerPullRequest` review comments over all its runs, and at most `maxPerInspection` comments of one inspection's
bytes await an answer at once.

## Logs

Each server run is named by its UTC start time, for example `2026-10-06T14-03-12.345Z`. When the server starts, it
prints its log folder as `Process log: <path>`.

| Path                                                                     | Contents                                          |
|--------------------------------------------------------------------------|---------------------------------------------------|
| `log/process-log/<run-id>/edict.log`                                     | INFO and above, also printed to stdout            |
| `log/process-log/<run-id>/edict-mcp.log`, `edict-mcp-system.log`         | Tool activity, with secrets redacted              |
| `log/process-log/<run-id>/edict-tasks.log`                               | Task progress and the price breakdown, if enabled |
| `log/process-log/<run-id>/intellij-mcp.log`, `intellij-mcp/log/idea.log` | IDE output and IDE logs                           |
| `log/agent-work/<run-id>/scratch/<task-id>`                              | Scratch space for each agent task                 |

Agents can't read `log/process-log`.

With `calculatePrice: true`, the manager reports the run's total cost when it finishes. The full breakdown goes to
`edict-price-report.json` in the process log folder. Costs are computed from the current OpenAI Standard rates.

## Security

The server is meant for a trusted local machine only. It listens on loopback, and it is not an authenticated
multi-user service.

Agents must not be able to write to the state root. Every state change goes through the server, which validates it.
Agents must not be able to read the server logs or Codex session transcripts either, because those hold other agents'
capability tokens and conversations. `install` sets this up for the default layout. Use `--deny` for any other paths
agents must not read.
