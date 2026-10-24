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
| `r` | Global | Refresh repository status |
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

## 🗺️ Roadmap & Slices

- [x] **VORQ-001 — Foundation:** Core architecture, domain models, CLI skeleton, CI pipeline.
- [x] **VORQ-002 — Discovery & Status:** Concurrent worktree inspection and porcelain status parsers.
- [x] **VORQ-003 — Workspace Provisioning:** `spawn` engine with declared file replication.
- [x] **VORQ-004 — Safe Teardown:** Multi-vector safety audit preventing data loss on uncommitted or ignored files.
- [x] **VORQ-005 — Guided Commits:** Per-file staging, deterministic Conventional Commits engine, and interactive CLI.
- [x] **VORQ-007 — Explicit Push:** Target confirmation and separated publication flow.
- [x] **VORQ-008 + VORQ-009 — Workspace Studio TUI:** Interactive Bubble Tea full-screen studio with file tree, diff viewer, and action modals.
- [ ] **VORQ-006 — Local LLM Adapter:** Optional offline Ollama integration for commit drafting.

---

## 📄 License

MIT © [Rafael Albernaz](https://github.com/Rafael-Albernaz-dev)
