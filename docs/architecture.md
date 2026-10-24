# vorq — System Architecture

`vorq` is an opinionated, local-first terminal workspace studio and Git companion built in Go. It guides developers and autonomous AI agents through clean workspace isolation (via Git Worktrees), interactive file browsing, syntax-highlighted diff inspection, local Conventional Commit drafting, and safe explicit publishing.

## 1. Architectural Principles

- **Unified Core (Single Point of Truth):** Both the Command Line Interface (CLI) and Terminal User Interface (TUI) invoke the exact same application use cases located in `internal/app/`. Neither interface implements custom business or Git logic.
- **Native Git Subprocesses:** The application owns typed repository ports; their native adapter constructs commands and parses output. Git operations are delegated directly to the native host `git` binary via structured arguments (`--porcelain -z`). This preserves user SSH keys, GPG signing, Git LFS, and custom hooks.
- **Git as Source of Truth:** Worktree discovery relies strictly on `git worktree list --porcelain -z`. The system avoids secondary state manifests that could drift out of sync.
- **Explicit Publishing:** Creating a commit never triggers a push. Publishing is a distinct, verifiable action with clearly stated remote and target branch.
- **Safety First:** Destructive operations (such as worktree removal) require checks on untracked changes, uncommitted modifications, and unversioned/ignored files (e.g. `.env`).

## 2. High-Level Component Diagram

```mermaid
flowchart TD
    User([Developer / Human]) --> UI_TUI[TUI (Bubble Tea)]
    Agent([AI Agent / Script]) --> UI_CLI[CLI (Cobra)]
    
    UI_TUI --> APP[Application Core Use Cases]
    UI_CLI --> APP
    
    subgraph Core ["internal/app & internal/domain"]
        APP --> SPAWN[Spawn Worktree]
        APP --> LIST[List & Discover Worktrees]
        APP --> COMMIT[Staged Diff & Commit Flow]
        APP --> PUSH[Explicit Push]
        APP --> REMOVE[Safe Teardown]
    end
    
    subgraph Adapters ["internal/git, internal/files, internal/suggest"]
        SPAWN --> GIT[Git Native Runner]
        LIST --> GIT
        COMMIT --> GIT
        PUSH --> GIT
        REMOVE --> GIT
        
        SPAWN --> FS[File Copier & Validator]
        
        COMMIT --> SUGGEST[Suggestion Engine]
        SUGGEST --> RULES[Deterministic Local Rules]
    end
    
    GIT --> OS_GIT[(Native Git CLI)]
    FS --> OS_FS[(File System)]
```

## 3. Directory Layout

```text
vorq/
├── cmd/
│   └── vorq/              # Application entrypoint & CLI routing
├── internal/
│   ├── domain/            # Core models: Project, Workspace, ChangeSet, CommitProposal
│   ├── app/               # Application use cases orchestrating workflows
│   ├── git/               # Native Git execution wrapper
│   ├── files/             # File synchronization, validation, and safety inspection
│   ├── suggest/           # Suggestion engine contracts (rules & local AI models)
│   ├── ui/
│   │   ├── cli/           # CLI commands, argument parsing, tabular renderers
│   │   └── tui/           # Bubble Tea terminal interface
│   └── version/           # Version and build metadata
├── docs/                  # Public technical documentation
├── go.mod
└── go.sum
```


## Presentation preferences and source highlighting

Appearance preferences belong to the user configuration directory, outside Git worktrees. Themes are saved by stable name through an atomic temporary-file rename; a revision guard prevents older commands in a session from overwriting newer choices. Closing the TUI flushes the latest selected theme.

Embedded Tree-sitter grammars supply lexical categories and local structural roles for Go, TS/TSX, JS/JSX, Python, HTML, CSS and JSON; parsers are scoped to the asynchronous preview command and closed after use. Recoverable incomplete source uses the grammar; unsupported or unavailable grammars retain the Chroma fallback. Chroma selects theme styles through its terminal256 formatter, with cached token styles and streaming output. There is no project execution, editor extension host, or cross-file language-server resolution. Parser bindings require CGO at build time. Git mutations continue exclusively through `app.Service`.


## Safety and application boundaries

`app.Repository` is a consumer-owned port composed of project, workspace, commit, explorer and push operations. It exposes typed metadata instead of `Exec` or native serialization. The composition root wraps the native runner in `git.Repository`; tests can inject failures at that boundary. CLI commands consume smaller workflow interfaces; the Studio contract excludes provisioning and teardown.

Worktree discovery uses at most four concurrent workers and joins them on cancellation. Failed status, ignored-file or required metadata reads propagate as errors. Safety audits refresh the status immediately before removal. `--force` authorizes known dirty data; it cannot bypass an incomplete audit or active/main-worktree protections. Git alone removes worktrees: any residual path, including a replaced directory or dangling symlink, is preserved and reported as an error.

Native stdout is capped at 16 MiB and stderr at 64 KiB; truncated stdout is never parsed as complete state. File/diff previews retain their separate 256 KiB cap. Porcelain parsers reject incomplete status records. Missing Git produces a CLI error while help/version remain available.

## Studio state and memory

The update loop owns separate navigation, commit, dialog, appearance, workspace and operation states. Commands, editing, modal input, messages and workspace context live in focused modules. Asynchronous commands capture immutable inputs and send results back with freshness tokens.

Only the active worktree retains source and rendered preview buffers. Inactive contexts preserve draft, selection and viewport positions, then reload data through the application when restored. Syntax output streams token fragments using cached terminal styles rather than formatting each source token through reflection. Rendered mode caches remain scoped to the active source/theme.
