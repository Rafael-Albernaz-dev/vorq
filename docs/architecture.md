# vorq — System Architecture

`vorq` is an opinionated, local-first terminal workspace studio and Git companion built in Go. It guides developers and autonomous AI agents through clean workspace isolation (via Git Worktrees), interactive file browsing, syntax-highlighted diff inspection, local Conventional Commit drafting, and safe explicit publishing.

## 1. Architectural Principles

- **Unified Core (Single Point of Truth):** Both the Command Line Interface (CLI) and Terminal User Interface (TUI) invoke the exact same application use cases located in `internal/app/`. Neither interface implements custom business or Git logic.
- **Native Git Subprocesses:** Git operations are delegated directly to the native host `git` binary via structured arguments (`--porcelain -z`). This preserves user SSH keys, GPG signing, Git LFS, and custom hooks.
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
        REMOVE --> FS
        
        COMMIT --> SUGGEST[Suggestion Engine]
        SUGGEST --> RULES[Deterministic Local Rules]
        SUGGEST --> LOCAL_AI[Local Model Endpoint (e.g., Ollama)]
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
│   ├── config/            # Project and local user settings
│   ├── suggest/           # Suggestion engine contracts (rules & local AI models)
│   ├── ui/
│   │   ├── cli/           # CLI commands, argument parsing, tabular renderers
│   │   └── tui/           # Bubble Tea terminal interface
│   └── version/           # Version and build metadata
├── docs/                  # Public technical documentation
├── go.mod
└── go.sum
```
