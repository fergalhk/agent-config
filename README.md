# agent-config

This repository holds the configuration for Fergal's coding agents.

## Contents

- `skills/<name>/` holds one Claude Code skill. A skill is first party or forked.
- `internal/config/skills.yaml` lists every skill and its type. A forked skill also names its upstream repository and directory.
- `internal/config/skill-forks.yaml` lists the upstream sources. The discovery job looks for new skills in these sources.
- `internal/forked-skills/<name>/last-commit-sha` records the last upstream commit that the fork includes.
- `docs/agents/skills/<name>.md` is the notes file for a forked skill. Claude Code reads it before each update.
- `output-styles/` holds output styles.
- `internal/cmd/skillctl` is the tool that the workflows run.

## How a forked skill flows

1. The discovery workflow runs each hour. It finds new skill directories in each source.
2. For each new skill, it opens a PR on the branch `skill-add/<name>`. The PR copies the skill with no change.
3. You merge the PR. The skill is now a forked skill.
4. The update workflow runs each hour. It finds the last upstream commit that changed the skill directory.
5. When upstream moves, the update job runs Claude Code. Claude Code applies the upstream change to the fork and keeps your changes. The job opens a PR on the branch `skill-update/<name>`.
6. When upstream moves again, the job rebases the same PR and updates it.
7. You merge the PR. The fork now records the new upstream commit.

If you close an update PR without a merge, the next run opens it again.

## Setup

The workflows use the built-in `GITHUB_TOKEN` for all GitHub operations. Do these steps once for the repository:

1. Add one repository secret: `ANTHROPIC_API_KEY`. The update workflows give it to Claude Code.
2. Go to Settings → Actions → General → Workflow permissions. Select "Allow GitHub Actions to create and approve pull requests". This setting is off by default. Without it, `GITHUB_TOKEN` cannot open a pull request, and the discovery and update jobs fail.
3. Keep the default workflow permissions as they are. Each workflow has its own `permissions` block. That block gives `GITHUB_TOKEN` the `contents: write` and `pull-requests: write` permissions. If you remove a `permissions` block, select "Read and write permissions" on the same page.

## Decline a new skill

Add the skill directory name to the `ignore` list of its source in `internal/config/skill-forks.yaml`. Then close the discovery PR. Discovery does not open a PR for that skill again.

## Steer Claude Code

You can tell Claude Code what to do in two ways:

- Comment on the update PR. A comment or a review starts a new update run for that skill. Claude Code reads your comment. Your comment has the highest authority.
- Edit the notes file `docs/agents/skills/<name>.md`. Claude Code follows it on each run. Use it for preferences that apply each time.

Claude Code can change only `skills/<name>/` and the notes file. A change to another file stops the run.

## skillctl

Run each subcommand from the repository root.

- `skillctl check` validates the configuration against the files in `skills/`. It prints one line for each problem.
- `skillctl list-forked` prints the names of the forked skills as a JSON list.
- `skillctl discover` runs the discovery job for all sources.
- `skillctl update --skill <name>` runs the update job for one forked skill.
- `skillctl skill-for-pr --pr <number>` prints the skill name of an update PR. It prints nothing for other PRs.

The `discover`, `update` and `skill-for-pr` subcommands need `GITHUB_TOKEN` and `GITHUB_REPOSITORY` in the environment.
