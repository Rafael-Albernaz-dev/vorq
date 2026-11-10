# vorq

[![Go Version](https://img.shields.io/badge/go-1.24%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![CI](https://github.com/Rafael-Albernaz-dev/vorq/actions/workflows/ci.yml/badge.svg)](https://github.com/Rafael-Albernaz-dev/vorq/actions)
[![Zero Electron](https://img.shields.io/badge/RAM-%3C%2030MB-brightgreen.svg)](README.md)
[![Startup](https://img.shields.io/badge/startup-%3C%2020ms-orange.svg)](README.md)

**A high-performance terminal workspace studio and Git companion in Go. The snappy, keyboard-driven alternative to heavy Electron IDEs for browsing files, inspecting diffs, managing isolated worktrees, and crafting safe commits.**

---

## 💡 The Real Problem (Why vorq?)

Modern development shouldn't require spinning up **1.5 GB+ RAM Electron applications** (such as VS Code) just to browse repository files, review colorized diffs, isolate branches, and craft conventional commits.

`vorq` was born out of frustration with bloated GUI editors and the friction of juggling multiple terminal windows and IDE tabs. Written in pure Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss):

- **Startup in `< 20ms`** (vs. 5–10 seconds for Electron).
- **RAM usage `< 30MB`** (vs. 1.2–2.0 GB).
- **Zero Electron dependencies** — runs directly inside any native terminal, over SSH, tmux, or local multiplexers.
- **Keyboard-first ergonomics** — manage your whole workspace without ever taking your hands off the home row.

---

## ⚡ Key Highlights

- **Workspace Studio TUI (`vorq up`):**
  - **Hierarchical File Tree:** VS Code-like tree explorer with Emoji (default) or ASCII language badges and real-time Git status badges (`+` staged, `~` modified, `?` untracked, `-` deleted).
  - **Unified Diff & File Viewer:** Inspect syntax-colored diffs (green additions, red removals) or full source files with line numbers. Scroll effortlessly with `j`/`k` or page keys.
  - **Instant 1-Key Staging (`s`):** Stage or unstage files asynchronously without freezing the UI. Use `A` / `U` for all files.
  - **Guided Conventional Commits (`c`):** Built-in deterministic heuristic proposing conventional commit messages (`feat:`, `fix:`, `chore:`) inferred from staged changes and directory scopes.
  - **Explicit Push Modal (`p`):** Visual confirmation showing `Local -> Remote` target and ahead/behind counts before touching the network.
  - **Repository Header:** Centered, credential-free link to the opened repository; click it or press `o` to open the browser.
  - **New Branch (`b`):** Review a name and base, then create and switch by default. Stage, working files and ignored files remain protected.
  - **Commit Graph (`H`):** Browse branches, tags, merges and commit patches; page through the local history and search/filter without automatic fetch.
  - **Pull Requests (`P`):** Explicit GitHub queries, checks, reviews, comments, and reviewed create/edit forms through authenticated `gh`.
  - **Live Worktree Switcher (`w`):** Switch between active workspaces and branches on the fly without exiting the studio.
- **Safe Worktree Lifecycle Management (`spawn`, `list`, `rm`):**
  - Provision isolated workspaces in dedicated directories, automatically replicating declared unversioned files (`.env`, config files).
  - Multi-vector safety audit: teardown strictly prevents deleting dirty, untracked, or ignored sensitive files without explicit `--force`.
- **Unified Hexagonal Architecture:**
  - Single source of truth in `internal/app/`. Both the CLI subcommands and the interactive TUI call the exact same application services.
- **Native Git Subprocesses:**
  - Uses native host `git` via structured arguments (`--porcelain -z`). Fully preserves SSH keys, GPG signing, Git LFS, and local pre-commit hooks.

---

## 🚀 Quick Start

### Installation

The embedded parsers require CGO and a C compiler (GCC or Clang) when building from source. Prebuilt binaries include the parsers and do not need editor extensions.

#### Using `go install` (Recommended):
```bash
go install github.com/Rafael-Albernaz-dev/vorq/cmd/vorq@latest
```

#### From source:
```bash
git clone https://github.com/Rafael-Albernaz-dev/vorq.git
cd vorq
go build -o bin/vorq ./cmd/vorq
sudo cp bin/vorq /usr/local/bin/ # or ~/.local/bin/
```

---

### Launching the Workspace Studio

Inside any Git repository directory, simply run:

```bash
vorq up
# or simply
vorq
```

To launch pointing to a specific repository path:
```bash
vorq up /path/to/another-project
```

---

The default icon mode is `emoji`; `--icons=text` selects ASCII language badges. Press `i` to switch modes; `t` opens appearance settings. The selected theme is saved per user in `$XDG_CONFIG_HOME/vorq/appearance.json` (normally `~/.config/vorq/appearance.json`) and restored on the next launch, across worktrees.

File and diff previews combine Chroma theme styles with embedded Tree-sitter parsers for Go, TypeScript/TSX, JavaScript/JSX, Python, HTML, CSS, and JSON. Functions, parameters, variables, types, properties, tags and attributes receive distinct roles. HTML script/style blocks and Go manifests are also highlighted. Local scope classification runs without executing project code; it does not provide cross-file type resolution or an IDE language server. Other languages and unavailable grammars retain Chroma highlighting. Plain terminal output is respected.

Discard opens a confirmation dialog with the exact path and impact. New staged files offer **Discard** (permanent deletion) and **Preserve** (`u`, unstage only). Ignored files, directories, symlinks, renames, conflicts, and submodules are protected. A changed file/index/HEAD invalidates the discard review. No discard runs without confirmation.

## ⌨️ Studio Keyboard Shortcuts

| Shortcut | Context | Action |
| :--- | :--- | :--- |
| `↑` / `↓` or `j` / `k` | Tree | Navigate files and directories |
| `Enter` or `Space` | Tree | Expand/collapse directory or select file |
| `Tab` | Global | Toggle between **Diff** and **Full File** view |
| `Shift + Tab` | Global | Switch focus between **File Tree** and **Viewer** |
| `↑` / `↓` or `j` / `k` | Viewer | Smooth scroll line by line |
| `PgUp` / `PgDown` | Viewer | Scroll page by page |
| `s` | Tree / Viewer | **Stage / Unstage** selected file asynchronously |
| `A` / `U` | Tree / Viewer | **Stage all / Unstage all**, preserving working files on unstage |
| `x` / `d` | Tree / Viewer | Review **Discard**; click `[Descartar]` in the viewer for the same dialog |
| `i` / `t` | Navigation | Switch Emoji / Text / appearance menu |
| `c` | Global | Open **Guided Commit** modal (type, heuristic message & editor) |
| `p` | Global | Open **Explicit Push** modal with visible remote target |
| `w` | Global | Open **Worktrees** switcher modal |
| `r` / **[↻ Atualizar]** | Global | Re-read the workspace. The Studio never refreshes on a timer: selection, scroll, open file and expanded folders stay put; vorq re-reads after its own actions and when you click **Atualizar** |
| `?` | Global | Toggle Help overlay |
| `Esc` | Modals | Cancel / close modal (preserving current stage) |
| `q` or `Ctrl + C` | Global | Exit and cleanly restore terminal |

---

Worktree removal blocks when status or ignored-file inspection fails, including with `--force`. If a path remains after Git finishes removal, vorq preserves it and reports the partial outcome. The active and main worktrees remain protected.

## 🛠️ CLI Subcommands (Scriptable & Non-Interactive)

For automated scripts, AI agents, or fast terminal commands:

```bash
# Verify environment health and native Git installation
vorq doctor

# List all active worktrees and their Git statuses
vorq list

# Provision an isolated Git worktree with automatic .env copy
vorq spawn feat/my-new-task --base main

# Review workspace status and staged/unstaged changes
vorq status

# Stage all and commit with conventional commit suggestion (or dry-run preview)
vorq commit -a --type feat
vorq commit --dry-run --type fix -a

# Explicitly push local commits to verified remote
vorq push --yes

# Safely teardown a worktree checking for uncommitted and ignored files
vorq rm feat/my-new-task
```

---

## 🏗️ Architecture

```
   Human (Interactive TUI)         AI Agent / CLI (Automated)
            │                                 │
            └───────────────┬─────────────────┘
                            │
                            ▼
              ┌───────────────────────────┐
              │     Application Core      │ ───► Heuristic Rules Engine
              │      (internal/app)       │      (internal/suggest)
              └───────────────────────────┘
                     │             │
                     ▼             ▼
               Native Git CLI  Filesystem
```

---

## AI commit suggestions

Open the **IA** tab and choose **Provedor e modelo**, **Sugerir mensagem**, **Revisar diff**, **Organizar commits**, or **Revisar plano**. For message suggestions, you can also click **Sugerir com IA** in the Commit panel, or **[IA] Sugerir** in the footer of any workspace view (keyboard: **Ctrl+G**). Choose **Codex** or **Gemini (Antigravity)**, select a model with the mouse or arrow keys, review the selected model, and confirm sending the staged diff. Review the suggested English Conventional Commit title and body. Choose **Usar** to fill the editor, or **Commit** to open the local commit review with that suggestion; only the subsequent confirmation creates the commit. Applying the preview replaces both draft fields; cancelling preserves your draft. Commit and push remain separate actions. Local rules remain available offline and preserve an existing description.

AI requires an official CLI installed and signed in with your own subscription: `codex login` using ChatGPT, or `agy` using your Google account. Gemini uses **Antigravity CLI (`agy`)**, not the legacy `gemini` command. Provider quotas and availability apply. The TUI model picker reads Codex’s official local model cache and discovers Gemini models through `agy models`. Codex also offers its CLI default. The CLI accepts `--model <id>`; without it, Gemini uses `gemini-3.8-flash-low` and Codex uses its CLI default. Model availability depends on the account and client catalog. Claude and Ollama are not integrated in this slice.

Only the staged diff is supplied as repository content. AI sends that code to the selected provider, so review the stage before sending. Diffs above 48 KiB are rejected before transmission rather than silently truncated. Clients run in a temporary empty workspace; the adapter disables Codex shell/browser/apps/hooks and isolates Antigravity settings with strict deny rules, no credit fallback, and a text-only agent definition. Provider clients may add their own system context. The Antigravity client still advertises built-in tools in its event metadata; the empty agent definition alone is not a sandbox. Unstaged/untracked files and project instructions are not supplied. vorq does not store API keys or reuse editor API endpoints.

Generation has a 90-second timeout and is cancellable. The stage is checked again after generation and before a TUI preview is applied. Failure offers retry, another provider, or local rules through **Opções**; providers and billing modes never switch automatically. Existing draft text is preserved until you explicitly apply a reviewed suggestion. Output is validated, bounded, and treated as text.

```bash
vorq suggest --provider codex
vorq suggest --provider codex --model gpt-6-luna
vorq suggest --provider gemini  # Gemini through Antigravity CLI
vorq suggest --provider rules --type fix
```

The explicit CLI command sends the staged diff immediately and prints a suggestion; it never stages files or creates a commit. Errors explain the same recovery choices as the TUI. Ctrl+C cancels the client process group.

## Branches, history and pull requests

The right panel has clickable **Commit / Git / PRs / Worktrees** tabs. Commit retains the message editor; Git offers branch creation, switching existing local branches, history, push review and branch updates; PRs offers list/create actions; Worktrees opens the existing workspace chooser. Switching tabs preserves the commit draft and performs no GitHub/network operation. Buttons open the same flows as keyboard shortcuts. In compact mode, click **Ações** in the top pane navigation first. With the right pane focused and outside text editing, Left/Right switches tabs and Up/Down/Tab selects an action; Enter opens it.

Click the **↑ ahead** counter beside the branch to review a push, or the **↓ behind** counter to review a fast-forward update. Neither click immediately transfers commits. Counts describe locally known refs and can be stale. Updating requires an attached branch, configured upstream and a clean workspace (including untracked files); ignored-file collisions are refused. After confirmation, vorq fetches the selected remote branch, checks HEAD/source/status again, and applies the fetched commit using fast-forward only. Divergence is refused; no merge commit, rebase, automatic stash or discard is performed. Fetch can update remote-tracking refs even if application is later blocked. The same workflow is available as `vorq pull`, with a displayed source and explicit `y` confirmation (`--yes` for scripts).

Press `b` in the Studio to create a branch, `H` for history, or `P` for PRs. Form input treats letters (including `q`) as text: `Tab` changes fields, `Ctrl+U` clears a field, `Ctrl+S` opens review, and `Enter` confirms from review. `Esc` cancels. Branch creation enters the new branch by default; choose another base only with a clean workspace. Ignored-file collisions are refused even with an otherwise clean status. New branches require an existing first commit.

Use **Git → Trocar branch** to select an existing local branch, review the source/destination, and confirm. The selector marks the active branch and branches checked out in another worktree; use the Worktrees tab for those. Switching requires a clean workspace, including staged, unstaged and untracked files. Ignored files remain protected against collisions. Source HEAD and target OID are checked again before switching. No stash, discard, remote-branch guessing or fetch runs. CLI equivalent: `vorq switch <branch>` (explicit confirmation, or `--yes`).

History shows the locally available Git graph, including a detached HEAD. Use `n` / `[` for next/previous pages, `/` to search messages or filter a ref, and `PgUp` / `PgDown` to scroll commit details. The shallow-clone indicator describes history availability; remote-tracking refs may be stale. No fetch runs automatically. Commit patches are previews capped at 256 KiB with a visible notice when truncated.

In history, `P` lists PRs and enriches matching head/merge commits with PR badges; `v` searches GitHub for PRs containing the selected commit. This also supports investigating squash/rebase histories without inventing a merge relationship from commit messages. PR lists default to all states and 100 results; `/` refines state/search and `n` expands the explicit limit up to 500. In the PR screen, `Enter` fetches details, `c` creates, and `e` edits. `Esc` returns to history or the commit workspace. Small terminals show a compact form with the active field; `Tab` still reaches every field.

GitHub features require [GitHub CLI](https://cli.github.com/) (`gh`) installed and authenticated with `gh auth login`. Git-only features work without it. This version supports `github.com` remotes; it does not manage tokens or create an authentication store. Remote selection prefers the current branch's upstream remote, then `origin`, then the sole remote; ambiguous or non-web remotes have no header link. SSH aliases are not guessed.

PR creation detects the base from the remote HEAD or GitHub default branch and drafts the title/body from local commits. Review title, multiline description, base, draft state, labels and reviewers before submitting. Editing supports title/body/base, adding labels/reviewers, and removing labels. Existing labels/reviewers remain unless explicitly changed. Creation requires the current HEAD already published under the same branch name on the selected remote; use the separate Push action first. A failed/timed-out PR submission can have a partial server result: inspect the list before retrying. Merge, rebase, cherry-pick, branch deletion and PR draft-state conversion are outside this workflow.

```bash
vorq branch feat/my-change             # create and enter
vorq branch feat/later --stay --base main
vorq tree --limit 100
vorq tree --skip 100 --limit 100
vorq tree --ref main --search 'fix:'
vorq tree --commit <full-commit-hash>
vorq pr list --state all --limit 100
vorq pr view 12
vorq pr create --base main --draft --title 'feat: my change' --body-file pr.md
vorq pr edit 12 --title 'Updated title' --body-file pr.md --label bug
```

PR writes display the repository, branches and fields before asking for `y`; EOF does not confirm. `--yes` is available for intentional noninteractive submission. Neither branch creation nor PR creation performs a push.

---

## 🗺️ Roadmap & Slices

- [x] **VORQ-001 — Foundation:** Core architecture, domain models, CLI skeleton, CI pipeline.
- [x] **VORQ-002 — Discovery & Status:** Concurrent worktree inspection and porcelain status parsers.
- [x] **VORQ-003 — Workspace Provisioning:** `spawn` engine with declared file replication.
- [x] **VORQ-004 — Safe Teardown:** Multi-vector safety audit preventing data loss on uncommitted or ignored files.
- [x] **VORQ-005 — Guided Commits:** Per-file staging, deterministic Conventional Commits engine, and interactive CLI.
- [x] **VORQ-007 — Explicit Push:** Target confirmation and separated publication flow.
- [x] **VORQ-008 — Workspace Studio TUI:** Interactive Bubble Tea full-screen studio with file tree, diff viewer, and action modals.
- [x] **VORQ-009 — Integrated Git Flow:** Repository link, branch creation, local commit graph and GitHub PR workflows in CLI/TUI.
- [ ] **VORQ-006 — Local LLM Adapter:** Optional offline Ollama integration for commit drafting.

---

## 📄 License

MIT © [Rafael Albernaz](https://github.com/Rafael-Albernaz-dev)


## AI review and approved commit plans

In the **IA** tab, **Revisar diff** reviews eligible working versions, including unstaged changes and new files. **Organizar commits** proposes ordered whole-file groups and English Conventional Commit messages. Both show a disclosure before sending repository content to the chosen subscription CLI. Ignored files, binaries, symlinks/submodules and files above 256 KiB are excluded. Analysis accepts up to 512 KiB of JSON input and rejects larger requests without silently truncating them.

Review every group and its **Diff** before execution. Arrow keys select a group; `e` edits its message, `m` moves a file ID to another group, `x` excludes a file, and `r` changes group order. Save edits with Ctrl+S. **Executar** opens a separate confirmation before creating the remaining local commits. A partially staged file contributes its complete reviewed working version, rather than only its staged portion. Hunk splitting is not supported.

The executor pins content, rechecks HEAD/index/files, uses a private Git index per commit, and preserves unrelated stage entries. Native hooks and signing still run. A failed sequence reports existing commit hashes and stops; **Revisar plano** reloads saved progress for resuming. Changes after review require a new plan. An `INSPECT:` warning requires inspecting history and index before proceeding. No automatic push, stash, reset or history rewrite is performed. Finish any merge/rebase/cherry-pick before using this workflow, and avoid concurrent commits in the same checkout.

Plans, including their pinned file contents, are stored privately under `~/.local/state/vorq/plans/` with owner-only file permissions. Provider/model selection belongs to the current TUI session. CLI equivalents:

```bash
vorq ai review --provider codex --model gpt-6-luna
vorq ai plan --provider gemini --model gemini-3.8-flash-low
vorq ai show --diff
vorq ai edit --kind message --group 1 --subject 'feat: describe reviewed changes'
vorq ai execute  # prints pinned diffs and asks for approval
```

Choose a model actually available in your CLI catalog. `ai execute --yes` explicitly approves the saved plan without prompting. `--json` output includes snapshot content and should be handled as private data. Plans suggest grouping and dependencies; they do not prove that each intermediate commit passes your tests.

## Repository inventory and file references

The **Repositório** tree includes tracked, untracked and ignored project files. Untracked names use cyan and `?`; ignored entries use gray and `!`, with a legend. Ignored directories expand on demand in pages of 200 entries. Counts describe loaded files; `…`/`+` and **Carregar mais…** indicate remaining content. Internal `.git` metadata is excluded. Viewing ignored files does not add them to **Alterações**, stage or AI input.

Right-click a file in the tree to copy its absolute path from the current worktree. The clicked row is used even when another file is selected, and symlink paths are copied without resolving their target. Clipboard support uses `wl-copy` on Wayland or `xclip`/`xsel` on X11; unavailable backends report an error. CLI: `vorq path internal/app/automation.go`, optionally with `--copy`.
