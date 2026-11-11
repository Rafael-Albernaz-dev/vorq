# vorq

[![Go Version](https://img.shields.io/badge/go-1.27%2B-blue.svg)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![CI](https://github.com/Rafael-Albernaz-dev/vorq/actions/workflows/ci.yml/badge.svg)](https://github.com/Rafael-Albernaz-dev/vorq/actions)

**A terminal workspace studio and Git companion in Go.** Browse files, read syntax-highlighted diffs, stage per file, write and review commits (with optional AI from your own subscriptions), manage worktrees, branches, history and GitHub pull requests — all from one keyboard- and mouse-friendly TUI, or from scriptable CLI commands that share the same core.

---

## Contents

- [Why vorq](#why-vorq)
- [Installation](#installation)
- [The Workspace Studio](#the-workspace-studio)
- [Commits, stage and discard](#commits-stage-and-discard)
- [Security checks](#security-checks)
- [Push, update, branches and history](#push-update-branches-and-history)
- [Pull requests](#pull-requests)
- [AI features](#ai-features)
- [Worktrees](#worktrees)
- [CLI reference](#cli-reference)
- [Safety model](#safety-model)
- [Architecture](#architecture)
- [Development](#development)
- [Roadmap](#roadmap)
- [License](#license)

---

## Why vorq

Reviewing changes, staging the right files and writing good commits should not require a multi-gigabyte editor. vorq runs in any terminal (local, SSH, tmux) and talks to your **native `git`**, so SSH keys, GPG signing, Git LFS and hooks keep working exactly as configured.

Measured on this repository with the release binary: `vorq list` finishes in about **40 ms using ~17 MB RAM**; the Studio draws its first full frame in about **40 ms using ~20 MB RAM**. Numbers vary with repository size and terminal.

Principles:

- **Explicit over implicit** — nothing is pushed, fetched, stashed, discarded or committed without an action you confirm.
- **Git is the source of truth** — status, worktrees and history always come from `git` itself (`--porcelain -z`).
- **One core, two interfaces** — the CLI and the TUI call the same use cases in `internal/app`.

---

## Installation

Requirements: **Go 1.27+**, `git`, and a C compiler (GCC or Clang) because the embedded Tree-sitter parsers use CGO. Optional: [`gh`](https://cli.github.com/) for pull requests, `wl-copy` or `xclip`/`xsel` for copying paths, and the official AI CLIs listed in [AI features](#ai-features).

```bash
go install github.com/Rafael-Albernaz-dev/vorq/cmd/vorq@latest
```

From source:

```bash
git clone https://github.com/Rafael-Albernaz-dev/vorq.git
cd vorq
go build -o bin/vorq ./cmd/vorq
cp bin/vorq ~/.local/bin/   # or any directory in PATH
```

Check the environment with `vorq doctor`.

---

## The Workspace Studio

Inside a Git repository run `vorq` (or `vorq up [path]`). The Studio interface text is in Portuguese; commit messages and PRs written by AI are in English.

**Layout** (wide terminals show all three panes; smaller ones show one at a time with **Arquivos / Conteúdo / Ações** tabs):

- **Header** — credential-free link to the repository (click or `o` to open in the browser).
- **Context row** — workspace path, branch, `↑ahead ↓behind` (click ↑ to review a push, ↓ to review an update) and the **[↻ Atualizar]** button.
- **Summary row** — counts of changed, staged, unstaged, new and conflicted files, plus the **Tela cheia [ ]** toggle at the right edge.
- **Left pane** — **Alterações** (changes grouped as staged / unstaged / new / conflicts) and **Repositório** (full inventory: tracked, untracked `?` in cyan, ignored `!` in gray; ignored folders load on demand in pages of 200).
- **Center pane** — viewer with **Trabalho** (index → working tree), **Stage** (HEAD → index) and **Arquivo** (full file) tabs, with Tree-sitter + Chroma highlighting for Go, TS/TSX, JS/JSX, Python, HTML, CSS and JSON.
- **Right pane** — **Commit / IA / Git / PRs / Worktrees** tabs with clickable actions.

**Refreshing.** The Studio never re-reads on a timer, so selection, scroll, the open file and expanded folders stay where you left them. It re-reads after its own actions; press `r` or click **[↻ Atualizar]** to pick up changes made elsewhere (the status bar reports `Workspace atualizado · HH:MM:SS`).

**Full screen.** Press `f` or click **Tela cheia [ ]** to read the current file or diff using the whole terminal. Inside: arrows/PgUp/PgDn scroll, `←`/`→` scroll horizontally, `z` toggles line wrap, `1`/`2`/`Tab` switch tabs, `s` stages and `x` discards (with confirmation). `Esc`, `f`, `q` or **Tela cheia [x]** return without re-reading.

**Appearance.** `t` opens themes (saved in `$XDG_CONFIG_HOME/vorq/appearance.json`), `i` toggles emoji/text file icons (`--icons text` at startup). Emoji icons are single-code-point glyphs so rows keep a stable width.

### Keyboard shortcuts

| Key | Where | Action |
| :--- | :--- | :--- |
| `↑`/`↓`, `j`/`k`, `PgUp`/`PgDn`, `g`/`G` | Tree / Viewer | Move selection or scroll |
| `Enter` / `Space` | Tree | Expand folder / open file in the viewer |
| `a` / `e` | Global | **Alterações** / **Repositório** views |
| `1` / `2` / `Tab` | Global | Viewer: Trabalho / Stage / toggle Arquivo |
| `Shift+Tab` | Global | Cycle focus: tree → viewer → right pane |
| `f` | Global | Full-screen viewer |
| `s` | Tree | Stage / unstage the selected file |
| `A` / `U` | Global | Stage all / unstage all (working files preserved) |
| `x` / `d` | Tree | Review a discard (always confirmed) |
| `c` | Global | Commit message editor; `c` again (or `Ctrl+S`) reviews the commit |
| `Ctrl+G` | Global / PR form | Suggest a commit message / write the PR with AI |
| `p` / `w` | Global | Review push / open worktrees |
| `b` / `H` / `P` | Global | New branch / commit history / pull requests |
| `r` | Global | Re-read the workspace |
| `t` / `i` / `?` | Global | Themes / icons / help |
| `Esc` | Dialogs | Cancel (stage and draft preserved) |
| `q` / `Ctrl+C` | Global | Quit (`Ctrl+C` also while editing) |

Mouse: click tabs, rows, buttons and the ↑/↓ counters; the wheel scrolls anywhere over a pane (it moves the list, not the selection); right-click a file to copy its absolute path.

---

## Commits, stage and discard

- **Stage per file** with `s` or by clicking the `[ ]` marker; `A` / `U` stage or unstage everything. Partially staged files are marked `[-]`.
- **Commit** — write a summary and description in the right pane, `Ctrl+S` opens a review of exactly what is staged, and only that confirmation creates the commit (hooks and signing run natively). A changed stage invalidates the review. Commits never push.
- **Local rules** (`Ctrl+G`, then `1`–`8`) propose a Conventional Commit offline from the staged files.
- **Discard** shows the exact path and impact first. New staged files offer **Descartar** (delete) or **Preservar** (`u`, unstage only). Ignored files, directories, symlinks, renames, conflicts and submodules are protected; a file/index/HEAD change invalidates the review.

---

## Security checks

Every commit path (Studio review, `vorq commit`, `commit -a`, commit plans), every push and every AI request runs the same local check. It uses built-in rules in Go (inspired by gitleaks), needs no network and no extra tools, and **never shows a detected value**: findings appear as `file:line` with a masked excerpt (`AKIA…LKJH`).

| Finding | Effect |
| :--- | :--- |
| Secrets: private keys, AWS/GitHub/GitLab/Slack/Stripe/Google/OpenAI/Anthropic/npm/SendGrid/Twilio tokens, JWTs, URLs with a password, hard-coded `password`/`secret`/`token`/`api_key` values, and files like `.env`, `*.pem`, `*.key`, `id_ed25519`, `.netrc`, `credentials.json`, `*.tfstate` | **blocks** |
| Merge conflict markers (`<<<<<<<` / `>>>>>>>`) | **blocks** |
| Added file above 5 MiB, forgotten debug code (`console.log`, `debugger`, `pdb.set_trace()`, …), paths differing only by letter case | warns, does not block |

- **Commit** — only what is staged is checked (added lines and new files). The review lists the findings; when something blocks, confirming asks you to type `CONFIRMAR`. CLI: the commit stops unless `--allow-secrets` is given, which then asks for `CONFIRMAR`; scripts must pass `--allow-secrets --yes`. An override applies to that single operation and is reported in the result.
- **Push** — before publishing, every commit that is not yet on any remote is checked one by one, so a secret added and removed later is still caught. A blocked push explains how to fix it (`git commit --amend`, `git rebase -i`, and revoking an exposed secret); vorq never rewrites history itself. The same typed override exists (`vorq push --allow-secrets`).
- **AI** — a file with a possible secret is left out **whole** (diff and content), listed as not sent, and never recommended for the stage or included in a plan. This applies to commit messages, review, next-commit advice, plans, context documents and PR drafts.
- **Exceptions** for false positives: add `vorq:allow-secret` on the line, or list paths and rules in a versioned `.vorqsecrets-allow` at the repository root:

```text
# path glob [rule ...]   (no rule = every rule; "*" = every path)
testdata/**   aws-access-key,jwt
.env.test
*             debug-statement
```

Globs without `/` match a file name at any depth; `**` matches any number of folders. Git hooks run outside vorq are not changed.

---

## Push, update, branches and history

- **Push** (`p` or click ↑) shows `Local → Remote/branch` and ahead count before sending; without upstream it proposes `origin` (or the only remote) and sets the upstream after confirmation. Never forced.
- **Update** (click ↓, or `vorq pull`) requires an upstream and a clean workspace, fetches only the reviewed branch into a private temporary ref, re-checks HEAD/source/status, then applies a **fast-forward only** merge. No merge commits, rebase, stash or overwriting of ignored files.
- **Bring main** (`[⇣ Trazer main]` beside `[↻ Atualizar]`, Git panel → *Trazer main*, or `vorq sync`) merges the remote default branch (`origin/HEAD`, else `main`/`master`) into the current branch. The review shows how many commits are known locally; confirming fetches only that branch into a private ref, re-checks HEAD/source/status and runs a native `git merge` (hooks and signing apply). It requires a clean workspace and never rebases, stashes or forces. Conflicts stop the merge with the files listed: resolve, stage and commit in the Studio, or abort (`a` in the dialog, `vorq sync --abort`) to restore the previous state. On the default branch itself the button opens the regular fast-forward update. Below 80 columns the button is hidden; use the Git panel.
- **Branches** (`b`) — create and enter a branch (stage and files preserved); another base requires a clean workspace. **Git → Trocar branch** switches to an existing local branch after review (clean workspace required; branches checked out in other worktrees are opened from the Worktrees tab).
- **Delete a branch** (**Git → Apagar branch**, or `d` / `[Apagar]` in the branch list; `vorq branch -d <name>`) reviews the branch first: whether it is merged into the default branch, how many commits are on no remote, and its remote branch, which is deleted **together** (pinned with `--force-with-lease`, so it is refused if someone pushed new commits). The current branch, the default branch and branches checked out in a worktree are refused. A branch that is not merged needs the typed `CONFIRMAR` (CLI: `--allow-unmerged`, scripts `--allow-unmerged --yes`); the result shows the hash to recover it (`git branch <name> <hash>`).
- **History** (`H`) — local commit graph with pages (`n` / `[`), search and ref filter (`/`) and patch previews (256 KiB cap, marked). No automatic fetch.
- **Credentials and signing.** The Studio never lets Git, SSH or GPG prompt over its screen. If a push, update, PR check, signed commit or plan needs a password, token, passphrase or PIN, it stops before publishing or committing and offers **[Terminal]** (`t`): the Studio is suspended, the same reviewed operation runs in your normal terminal where the prompt appears, and the Studio returns with the result. Cached credentials (credential helper, ssh-agent, gpg-agent) keep working without prompts. CLI commands prompt normally.

---

## Pull requests

Requires `gh` installed and authenticated (`gh auth login`) and a `github.com` remote. Remote selection: the branch's upstream remote, else `origin`, else the only remote.

- **List / view** (`P`) — PRs with state (`DRAFT` marked), checks, reviews and comments; `/` filters, `n` raises the limit up to 500. In history, `P` adds PR badges to commits and `v` finds PRs containing the selected commit.
- **Create / edit** (`c` / `e` on the PR screen) — a form with title, description, base, labels, reviewers and draft state, a review step, and an explicit confirmation. Creation requires the current HEAD to be already published on the remote (use Push first); vorq never pushes implicitly.
- **Write it with AI** — in the new-PR form press `Ctrl+G` or click **[IA]** (CLI: `vorq pr create --ai --provider claude`). The provider receives the branch commits and the per-file diff since the merge base with the locally known base (no fetch; up to 300 files and 2 MiB, files above 1 MiB replaced by a visible notice), the repository PR template when present and the tracked context documents. It writes an English title and a Markdown description (summary, motivation, changes, how to test, risks, breaking changes, reviewer notes) or fills your template, without claiming tests it cannot see. Only title and description are filled; a HEAD change during the analysis discards the result.
- **Mark ready** — PRs are created as drafts by default and GitHub does not merge drafts. On the PR screen press `y` on an open draft (or run `vorq pr ready <number>`) and confirm to mark it ready for review. Merging stays on GitHub.

A failed or timed-out submission may still have reached GitHub; check the list before retrying.

---

## AI features

AI is optional and uses **your own subscriptions** through the official clients — vorq stores no API keys and never falls back to paid API usage or to another provider.

| Provider | Client and login | Models offered |
| :--- | :--- | :--- |
| Codex | `codex` with `codex login` (ChatGPT) | CLI default + models from Codex's local catalog (`gpt-…`) |
| Gemini | Antigravity CLI `agy`, signed in with Google | discovered with `agy models` (`gemini-…`) |
| Claude Code | `claude`, then `/login` with your Claude subscription | default + `fable`, `opus`, `sonnet`, `haiku` aliases (or a full `claude-…` ID) |

Choose the provider in **IA → Provedor e modelo** (keys `c`, `g`, `a` in provider menus) or with `--provider` / `--model` in the CLI. Every request shows what will be sent before it leaves the machine.

| Feature | Studio | CLI | Input sent |
| :--- | :--- | :--- | :--- |
| Commit message | `Ctrl+G`, **Sugerir com IA** | `vorq suggest` | staged diff only, up to 256 KiB |
| Review | **IA → Revisar diff** | `vorq ai review` | eligible working changes (staged or not) |
| Prepare next commit | **IA → Preparar próximo commit** | `vorq ai stage [--apply]` | working changes + tracked context documents |
| Commit plan | **IA → Organizar commits / Revisar plano** | `vorq ai plan`, `show`, `edit`, `execute` | eligible working changes |
| Pull request | PR form `Ctrl+G` / **[IA]** | `vorq pr create --ai` | branch commits + merge-base diff + PR template + context documents |

**Review** — a thorough review (bugs, edge cases, error handling, security, performance, compatibility, missing tests), shown as a summary, severity counts and one card per finding with quoted evidence and a suggested fix, most severe first. Findings without valid evidence are dropped and counted.

**Prepare next commit** — the AI lists what to **stage** (with reasons) and what to **keep out** (with reasons and risks such as debug code, secrets, generated files, unrelated changes or missing tests required by your docs), plus a message. **[Aplicar]** lists every index change and, after confirmation, makes the stage exactly match the advice. Only the index changes; working files are never modified and nothing is committed.

**Commit plan** — ordered whole-file commit groups with messages. Review and edit groups (`e` message, `m` move, `x` exclude, `r` reorder, `Ctrl+S` save), then **Executar** confirms. Each commit uses a pinned snapshot and a private index, runs native hooks and signing, and stops on the first failure; progress is saved under `~/.local/state/vorq/plans/` (owner-only) so the plan can be resumed. Hunk splitting is not supported.

**What is sent and what is not.** Files with a possible secret ([Security checks](#security-checks)), ignored files, binaries, symlinks/submodules and files above 1 MiB are never sent (they are listed as not analysed). Context documents are only **tracked** files — root README/CONTRIBUTING, `docs/*.md`, `.github` templates and commit conventions (`.gitmessage`, commitlint), up to 32 KiB each and 96 KiB in total with truncation marked; untracked or ignored notes are never used as context. Analyses accept up to 2 MiB and are refused (never silently truncated) above the limits.

**Isolation.** Clients run in an empty temporary directory with a minimal environment (no API keys, no parent agent variables). Codex runs with its shell, browser, apps and hooks disabled; Antigravity with a private home and strict deny rules; Claude Code with `claude -p --tools ""`, no MCP servers, no slash commands and no session persistence, its output enforced by `--json-schema` (vorq does not use `--bare` because that mode only accepts API keys). Responses are validated and treated as text. Timeouts: 3 minutes for messages, 5 minutes for analyses; `Esc` / `Ctrl+C` cancel the client process group. Errors include a short sanitised excerpt of the client's own message (for example "Not logged in" or quota problems).

---

## Worktrees

- `vorq spawn <branch>` creates an isolated worktree in `../<repo>-worktrees/<branch>` from the repository default branch (origin/HEAD, then `main`/`master`, else the current HEAD; printed), copying `.env.local`/`.env` (or `.env.example` as `.env`). `--copy <file>` adds workspace-relative regular files; `..`, absolute paths, `.git`, symlinks and missing files are rejected before anything is created.
- `vorq list` and the **Worktrees** tab show every worktree with status and ahead/behind (branches whose upstream was deleted are shown normally).
- `vorq rm <worktree>` refuses to remove the main or active worktree, and blocks on staged, unstaged, untracked or ignored files unless `--force`. The branch is kept.

---

## CLI reference

```bash
vorq                      # open the Studio (same as: vorq up [path])
vorq doctor               # check git, repository and environment
vorq status [workspace]   # changes and diffs
vorq list                 # worktrees

vorq commit                       # guided: stage, type, message, confirm
vorq commit -a -m 'feat: x'       # non-interactive
vorq commit --dry-run --type fix  # passive preview
vorq commit -m 'x' --allow-secrets [--yes]  # override blocking findings (typed CONFIRMAR, or --yes in scripts)
vorq suggest --provider claude    # AI message from the staged diff (or --provider rules)

vorq push [--yes] [--allow-secrets]  # reviewed push; unpublished commits are checked first
vorq pull [--yes]                 # fetch + fast-forward only
vorq sync [--yes]                 # fetch + merge the remote default branch (main) into the current branch
vorq sync --abort                 # abort a merge stopped by conflicts
vorq branch feat/x [--stay --base main]
vorq branch -d feat/x [--yes] [--allow-unmerged]  # delete local + remote branch after review
vorq switch <branch> [--yes]
vorq tree [--limit 100 --skip 0 --ref main --search 'fix:' --commit <hash>]

vorq pr list [--state all --limit 100]
vorq pr view <n>
vorq pr create [--base main --title ... --body-file pr.md --draft=false] [--ai --provider claude]
vorq pr edit <n> [--title ... --label bug --remove-label wip]
vorq pr ready <n> [--yes]

vorq ai review [--provider codex|gemini|claude --model ...]
vorq ai stage [--apply --yes]
vorq ai plan | show [--diff] | edit ... | execute [--yes]

vorq spawn <branch> [--base ... --dir ... --copy file]
vorq rm <worktree> [--force]
vorq prune
vorq path <file> [--copy]
```

Every command that writes (commit, push, pull, switch, PR, plan execution, stage advice) shows its target and asks for confirmation; `--yes` exists for intentional scripted use and EOF never confirms. `--json` outputs of AI commands include file contents — treat them as private.

---

## Safety model

- No implicit network: fetch, push and GitHub calls only run from explicit actions.
- No implicit history changes: no stash, reset, rebase, force push or merge commits.
- Reviewed state is re-validated before writing (stage diff, HEAD, push destination, plan snapshot, discard fingerprint); anything that changed requires a new review.
- Read-only Studio queries use `--no-optional-locks` and never rewrite `.git/index`.
- Ignored and sensitive files are protected from discard, worktree removal and AI input.
- Secrets and conflict markers block commits and pushes until fixed or explicitly overridden with a typed confirmation; files with possible secrets are never sent to AI providers ([Security checks](#security-checks)).

---

## Architecture

```
  Studio TUI (Bubble Tea)          CLI (Cobra)
            └──────────────┬──────────────┘
                           ▼
                internal/app  (use cases, rules)
                  │       │        │
        ports ────┘       │        └──── ports
          ▼               ▼              ▼
  internal/git     internal/github   internal/suggest
  native git        gh CLI            official AI CLIs + local rules
```

- `internal/app` owns every rule (branch, remote selection, safety, AI validation) and declares the ports it needs (`app.Repository`, `app.AIProvider`, `app.PullRequestProvider`). Adapters are checked against them at compile time, so a missing capability is a build error.
- `internal/git` runs `git` with structured arguments and parses `--porcelain -z` output; it holds no business policy.
- `internal/domain` holds shared types; `internal/ui/tui` and `internal/ui/cli` are presentation only.

More detail: [docs/architecture.md](docs/architecture.md).

---

## Development

```bash
go build ./...
go vet ./...
gofmt -l cmd internal                      # must print nothing
go run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./...
go test -race ./...
```

CI runs the same gates on every push and pull request. Tests use real temporary Git repositories; AI and GitHub clients are replaced by fakes (the opt-in live provider test is skipped by default).

---

## Roadmap

Delivered:

- **Foundation & discovery** — core, porcelain parsers, worktree inspection.
- **Worktree lifecycle** — `spawn`, safe `rm`, `prune`.
- **Guided commits & safe discard**, **explicit push**, **fast-forward update**, **merge from the default branch**.
- **Workspace Studio** — three-pane TUI, highlighting, mouse, themes, full-screen viewer, explicit refresh.
- **Integrated Git flow** — repository link, branches (create, switch, delete), history graph, GitHub PRs (create, edit, mark ready).
- **AI by subscription** — Codex, Gemini (Antigravity) and Claude Code for commit messages, reviews, next-commit staging advice, commit plans and PR descriptions.
- **Hardening** — credential/GPG prompts kept out of the Studio, upstream-gone tolerance, lint gates in CI, explicit ports.
- **Security checks** — secrets and conflict markers block commits and pushes (typed override), files with possible secrets never reach AI providers.

Not planned for now: hunk-level staging, local LLMs.

---

## License

MIT © [Rafael Albernaz](https://github.com/Rafael-Albernaz-dev)
