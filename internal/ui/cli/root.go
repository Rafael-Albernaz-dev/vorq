package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
	"github.com/Rafael-Albernaz-dev/vorq/internal/github"
	"github.com/Rafael-Albernaz-dev/vorq/internal/suggest"
	"github.com/Rafael-Albernaz-dev/vorq/internal/ui/tui"
	"github.com/Rafael-Albernaz-dev/vorq/internal/version"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))

	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#04B575"))

	warnStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFB86C"))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF5555"))

	branchStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#8BE9FD"))

	activeMarkerStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#50FA7B"))

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#BD93F9"))
)

// cell pads by visible terminal width (not bytes), so ANSI colors and
// multi-byte characters never misalign table columns.
func cell(s string, width int) string {
	if w := ansi.StringWidth(s); w > width {
		return ansi.Truncate(s, width, "…")
	} else {
		return s + strings.Repeat(" ", width-w)
	}
}

// Composition-root contract checks (VORQ-017): the native adapters must
// provide every capability the application core declares.
var (
	_ app.Repository = (*git.Repository)(nil)
	_ app.AIProvider = (*suggest.CLIProvider)(nil)
	_ git.Runner     = (*git.NativeRunner)(nil)
)

// NewRootCmd initializes the root Cobra command and its subcommands.
func NewRootCmd() *cobra.Command {
	var showVersion bool
	var icons string
	var appCore *app.App

	rootCmd := &cobra.Command{
		Use:   "vorq",
		Short: "A local-first companion for safe Git worktree workflows and guided commits",
		Long: titleStyle.Render("vorq") + " — " +
			subtleStyle.Render("Opinionated, local-first CLI and TUI companion in Go for humans and AI agents.") + "\n\n" +
			"Execute " + branchStyle.Render("vorq up") + " (ou apenas " + branchStyle.Render("vorq") + ") para subir o Workspace Studio em tela cheia.\n" +
			"Manage Git Worktrees, inspect staged diffs, and create safe commits without friction.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Fprintf(cmd.OutOrStdout(), "vorq version %s (commit: %s, built: %s)\n",
					version.Version, version.GitCommit, version.BuildDate)
				return nil
			}
			return runStudio(cmd, appCore, ".", icons)
		},
	}

	// Runtime errors print "Error: ..." only; usage is for --help and bad flags.
	rootCmd.SilenceUsage = true
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show vorq version")

	rootCmd.PersistentFlags().StringVar(&icons, "icons", "emoji", "Ícones da TUI: emoji ou text")

	// Wire application core
	gitRunner, err := git.NewNativeRunner()
	if err != nil {
		rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if cmd == rootCmd && showVersion {
				return nil
			}
			return err
		}
	} else {
		appCore = app.NewApp(git.NewRepository(gitRunner))
		appCore.SetPullRequestProvider(github.NewClient())
		appCore.SetAIProvider(suggest.NewCLIProvider())
	}

	rootCmd.AddCommand(newUpCmd(appCore, &icons))

	// Add subcommands
	rootCmd.AddCommand(newDoctorCmd(appCore))
	rootCmd.AddCommand(newListCmd(appCore))
	rootCmd.AddCommand(newSpawnCmd(appCore))
	rootCmd.AddCommand(newStatusCmd(appCore))
	rootCmd.AddCommand(newCommitCmd(appCore))
	rootCmd.AddCommand(newPushCmd(appCore))
	rootCmd.AddCommand(newPullCmd(appCore))
	rootCmd.AddCommand(newSyncCmd(appCore))
	rootCmd.AddCommand(newSwitchCmd(appCore))
	rootCmd.AddCommand(newSuggestCmd(appCore))
	rootCmd.AddCommand(newAICmd(appCore), newPathCmd(appCore))
	rootCmd.AddCommand(newRmCmd(appCore))
	rootCmd.AddCommand(newPruneCmd(appCore))
	rootCmd.AddCommand(newBranchCmd(appCore), newTreeCmd(appCore), newPRCmd(appCore))

	return rootCmd
}

// Execute runs the root command.
func Execute() {
	// Ctrl+C/SIGTERM cancel the command context, so native Git subprocesses
	// are stopped instead of being orphaned.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := NewRootCmd().ExecuteContext(ctx); err != nil {
		stop()
		os.Exit(1)
	}
}

func newUpCmd(appCore app.StudioService, icons *string) *cobra.Command {
	return &cobra.Command{
		Use:     "up [dir]",
		Aliases: []string{"tui", "studio", "open"},
		Short:   "Subir o Workspace Studio em tela cheia (árvore de arquivos, diffs e commits)",
		Long: "Inicia o Workspace Studio no diretório atual (ou no caminho especificado).\n" +
			"Navegue pelos arquivos, visualize diffs coloridos, organize worktrees e faça commits guiados.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}
			return runStudio(cmd, appCore, targetDir, *icons)
		},
	}
}

func runStudio(cmd *cobra.Command, core app.StudioService, dir string, icons string) error {
	mode, err := tui.ParseIconMode(icons)
	if err != nil {
		return err
	}
	return tui.Run(cmd.Context(), core, dir, cmd.InOrStdin(), cmd.OutOrStdout(), mode)
}

func newDoctorCmd(appCore app.ProjectService) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Verify environment health, native Git installation, and current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			ctx := cmd.Context()

			gitVer, err := appCore.GetGitVersion(ctx)
			if err != nil {
				return fmt.Errorf("git check failed: %w", err)
			}
			fmt.Fprintln(out, successStyle.Render("✓")+" Native Git: "+gitVer)

			proj, err := appCore.ResolveProject(ctx, ".")
			if err != nil {
				fmt.Fprintln(out, subtleStyle.Render("ℹ Current directory is not a Git repository (or in a bare tree)."))
			} else {
				fmt.Fprintln(out, successStyle.Render("✓")+" Git Common Dir: "+proj.GitDir)
				fmt.Fprintln(out, successStyle.Render("✓")+" Git Root: "+proj.RootPath)
			}

			fmt.Fprintln(out, successStyle.Render("✓")+" vorq engine initialized successfully.")
			return nil
		},
	}
}

func newListCmd(appCore interface {
	app.ProjectService
	app.WorkspaceReader
}) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List active worktrees and their Git statuses",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			ctx := cmd.Context()

			worktrees, err := appCore.ListWorktrees(ctx, ".")
			if err != nil {
				return err
			}

			if len(worktrees) == 0 {
				fmt.Fprintln(out, subtleStyle.Render("No worktrees found."))
				return nil
			}

			cwd, _ := os.Getwd()
			proj, _ := appCore.ResolveProject(ctx, ".")

			fmt.Fprintln(out)
			fmt.Fprintf(out, "%s %s %s %s %s\n",
				cell("", 2),
				cell(headerStyle.Render("BRANCH"), 24),
				cell(headerStyle.Render("PATH"), 32),
				cell(headerStyle.Render("COMMIT"), 10),
				headerStyle.Render("STATUS"),
			)
			fmt.Fprintln(out, subtleStyle.Render(strings.Repeat("─", 2+1+24+1+32+1+10+1+6)))

			for _, ws := range worktrees {
				marker := "  "
				if ws.IsCurrent {
					marker = activeMarkerStyle.Render("● ")
				} else if ws.IsMain {
					marker = subtleStyle.Render("○ ")
				}

				var branchName string
				if ws.IsMain {
					branchName = branchStyle.Render(ws.Branch) + subtleStyle.Render(" (main)")
				} else {
					branchName = branchStyle.Render(ws.Branch)
				}

				// Relative display path
				displayPath := ws.Path
				if proj != nil {
					if ws.Path == proj.RootPath {
						displayPath = "./"
					} else if rel, err := filepath.Rel(filepath.Dir(proj.RootPath), ws.Path); err == nil && !strings.HasPrefix(rel, "..") {
						displayPath = "../" + rel
					} else if rel, err := filepath.Rel(cwd, ws.Path); err == nil {
						displayPath = rel
					}
				}

				shortCommit := ws.CommitHash
				if len(shortCommit) > 7 {
					shortCommit = shortCommit[:7]
				}

				// Status badges
				var statusBadge string
				if ws.IsLocked {
					statusBadge = errorStyle.Render("🔒 locked")
					if ws.LockReason != "" {
						statusBadge += subtleStyle.Render(" (" + ws.LockReason + ")")
					}
				} else if ws.Status != nil {
					if ws.Status.IsClean() {
						statusBadge = successStyle.Render("✓ clean")
					} else {
						var parts []string
						if st := ws.Status.StagedCount(); st > 0 {
							parts = append(parts, successStyle.Render(fmt.Sprintf("+%d", st)))
						}
						if unst := ws.Status.UnstagedCount(); unst > 0 {
							parts = append(parts, warnStyle.Render(fmt.Sprintf("~%d", unst)))
						}
						if untr := ws.Status.UntrackedCount(); untr > 0 {
							parts = append(parts, subtleStyle.Render(fmt.Sprintf("?%d", untr)))
						}
						statusBadge = warnStyle.Render("dirty") + " (" + strings.Join(parts, " ") + ")"
					}

					if ws.Status.Ahead > 0 || ws.Status.Behind > 0 {
						syncPart := ""
						if ws.Status.Ahead > 0 {
							syncPart += fmt.Sprintf("↑%d", ws.Status.Ahead)
						}
						if ws.Status.Behind > 0 {
							syncPart += fmt.Sprintf("↓%d", ws.Status.Behind)
						}
						statusBadge += " " + subtleStyle.Render(syncPart)
					}
				} else {
					statusBadge = subtleStyle.Render("unknown")
				}

				fmt.Fprintf(out, "%s %s %s %s %s\n",
					cell(marker, 2),
					cell(branchName, 24),
					cell(subtleStyle.Render(displayPath), 32),
					cell(subtleStyle.Render(shortCommit), 10),
					statusBadge,
				)
			}
			fmt.Fprintln(out)
			return nil
		},
	}
}

func newStatusCmd(appCore app.WorkspaceReader) *cobra.Command {
	return &cobra.Command{
		Use:   "status [workspace]",
		Short: "Display workspace status, changes, and staged diffs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			ctx := cmd.Context()
			target := ""
			if len(args) > 0 {
				target = args[0]
			}

			ws, cs, err := appCore.GetWorkspaceStatus(ctx, ".", target)
			if err != nil {
				return err
			}

			fmt.Fprintln(out)
			header := fmt.Sprintf("Workspace: %s  |  Branch: %s",
				titleStyle.Render(ws.Path),
				branchStyle.Render(ws.Branch),
			)
			if ws.IsMain {
				header += subtleStyle.Render(" [main tree]")
			}
			fmt.Fprintln(out, header)

			if len(ws.CommitHash) > 7 {
				fmt.Fprintf(out, "HEAD: %s", subtleStyle.Render(ws.CommitHash[:7]))
				if cs.Ahead > 0 || cs.Behind > 0 {
					fmt.Fprintf(out, " (ahead %d, behind %d)", cs.Ahead, cs.Behind)
				}
				fmt.Fprintln(out)
			}
			fmt.Fprintln(out)

			if cs.IsClean() {
				fmt.Fprintln(out, successStyle.Render("✓ Working tree clean (no staged or unstaged changes)"))
				fmt.Fprintln(out)
				return nil
			}

			// Group files into Staged, Unstaged, and Untracked
			var stagedFiles []domain.FileStatus
			var unstagedFiles []domain.FileStatus
			var untrackedFiles []domain.FileStatus

			for _, f := range cs.Files {
				if f.Staged == "?" || f.Unstaged == "?" {
					untrackedFiles = append(untrackedFiles, f)
					continue
				}
				if f.Staged != "" && f.Staged != " " {
					stagedFiles = append(stagedFiles, f)
				}
				if f.Unstaged != "" && f.Unstaged != " " {
					unstagedFiles = append(unstagedFiles, f)
				}
			}

			if len(stagedFiles) > 0 {
				fmt.Fprintln(out, successStyle.Render("Changes to be committed:")+subtleStyle.Render(" (staged for next commit)"))
				for _, f := range stagedFiles {
					pathStr := f.Path
					if f.OrigPath != "" {
						pathStr = f.OrigPath + " -> " + f.Path
					}
					fmt.Fprintf(out, "  %s  %s\n", successStyle.Render(f.Staged), pathStr)
				}
				fmt.Fprintln(out)
			}

			if len(unstagedFiles) > 0 {
				fmt.Fprintln(out, warnStyle.Render("Changes not staged for commit:")+subtleStyle.Render(" (use 'git add <file>...')"))
				for _, f := range unstagedFiles {
					fmt.Fprintf(out, "  %s  %s\n", warnStyle.Render(f.Unstaged), f.Path)
				}
				fmt.Fprintln(out)
			}

			if len(untrackedFiles) > 0 {
				fmt.Fprintln(out, errorStyle.Render("Untracked files:")+subtleStyle.Render(" (unversioned files)"))
				for _, f := range untrackedFiles {
					fmt.Fprintf(out, "  ?  %s\n", subtleStyle.Render(f.Path))
				}
				fmt.Fprintln(out)
			}

			return nil
		},
	}
}

func newSpawnCmd(appCore app.WorkspaceService) *cobra.Command {
	var baseBranch string
	var targetDir string
	var checkoutExisting bool
	var copyFiles []string

	cmd := &cobra.Command{
		Use:   "spawn <branch-name>",
		Short: "Create an isolated Git worktree and copy declared local files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			branch := args[0]
			ctx := cmd.Context()

			res, err := appCore.SpawnWorkspace(ctx, ".", app.SpawnOptions{
				BranchName:       branch,
				BaseBranch:       baseBranch,
				TargetDir:        targetDir,
				CheckoutExisting: checkoutExisting,
				CopyFiles:        copyFiles,
			})
			if err != nil {
				return err
			}

			fmt.Fprintln(out)
			fmt.Fprintf(out, "%s Workspace provisioned successfully!\n", successStyle.Render("✓"))
			fmt.Fprintln(out)
			fmt.Fprintf(out, "  %-10s %s\n", subtleStyle.Render("Branch:"), branchStyle.Render(res.Workspace.Branch))
			if res.BaseBranch != "" {
				fmt.Fprintf(out, "  %-10s %s\n", subtleStyle.Render("Base:"), branchStyle.Render(res.BaseBranch))
			}
			fmt.Fprintf(out, "  %-10s %s\n", subtleStyle.Render("Path:"), titleStyle.Render(res.Workspace.Path))
			if len(res.CopiedFiles) > 0 {
				fmt.Fprintf(out, "  %-10s %s\n", subtleStyle.Render("Copied:"), successStyle.Render(strings.Join(res.CopiedFiles, ", ")))
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, subtleStyle.Render("To switch to your new workspace:"))
			fmt.Fprintf(out, "  cd %s\n\n", res.Workspace.Path)
			return nil
		},
	}

	cmd.Flags().StringVarP(&baseBranch, "base", "b", "", "Base branch (default: repository default branch — origin/HEAD, main or master — else current HEAD)")
	cmd.Flags().StringVarP(&targetDir, "dir", "d", "", "Custom destination directory for the worktree")
	cmd.Flags().BoolVarP(&checkoutExisting, "checkout", "c", false, "Checkout existing branch instead of creating new")
	cmd.Flags().StringSliceVar(&copyFiles, "copy", nil, "Extra files to copy from the main workspace (can be repeated)")
	return cmd
}

func newCommitCmd(appCore interface {
	app.CommitService
	app.StageService
}) *cobra.Command {
	var all, dryRun, allowSecrets, yes bool
	var commitType, message string
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Stage files and create a commit guided by local message proposals",
		Long: "Cria um commit local preservando hooks e assinatura do Git, sem push.\n" +
			"Sem flags, permite stage seletivo, escolha de tipo, edição e confirmação.\n" +
			"--message dispensa confirmação e requer stage existente ou --all.\n" +
			"--type pula apenas o menu de tipos; a confirmação continua obrigatória.\n" +
			"--dry-run é passivo: não altera o índice nem o histórico. Com --all,\n" +
			"prevê as alterações contra HEAD, incluindo untracked não ignorados.\n" +
			"Em dry-run, o tipo padrão é chore quando --type não é informado.\n" +
			"Cancelar após stage preserva os arquivos em stage.\n" +
			"Verificação de segurança: segredos (chaves, tokens, senhas, .env) e\n" +
			"marcadores de conflito no stage bloqueiam o commit; arquivo > 5 MiB,\n" +
			"debug esquecido e colisão de maiúsculas/minúsculas só avisam.\n" +
			"--allow-secrets permite seguir após digitar CONFIRMAR (scripts: --yes).\n" +
			"Exceções: comentário vorq:allow-secret na linha ou .vorqsecrets-allow.",
		Example: "  vorq commit\n  vorq commit --type fix\n  vorq commit -a -m 'feat: add guided commits'\n  vorq commit --dry-run --type feat -a\n  vorq commit -m 'test: add fixtures' --allow-secrets --yes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			reader := bufio.NewReader(cmd.InOrStdin())
			manual := cmd.Flags().Changed("message")
			if manual && strings.TrimSpace(message) == "" {
				return fmt.Errorf("the commit message must not be empty")
			}
			if cmd.Flags().Changed("type") {
				if err := app.ValidateCommitType(commitType); err != nil {
					return err
				}
			}
			cs, err := appCore.GetCommitChanges(ctx, ".")
			if err != nil {
				return err
			}
			if cs.IsClean() {
				fmt.Fprintln(out, successStyle.Render("✓ Working tree clean (nada para comitar)"))
				return nil
			}
			if dryRun {
				previewType := commitType
				if previewType == "" {
					previewType = "chore"
				}
				preview, proposal, err := appCore.PreviewCommit(ctx, ".", previewType, all)
				if err != nil {
					return fmt.Errorf("dry-run: %w (use --all to include unstaged changes)", err)
				}
				proposedMessage := proposal.Message()
				if manual {
					proposedMessage = message
				}
				fmt.Fprintln(out, titleStyle.Render("[DRY-RUN] Proposta de commit: ")+branchStyle.Render(proposedMessage))
				for _, file := range preview.Files {
					fmt.Fprintf(out, "  %s %q\n", successStyle.Render(file.Staged), file.Path)
				}
				fmt.Fprintln(out, subtleStyle.Render("[DRY-RUN] Nenhum stage ou commit executado."))
				return nil
			}
			if all {
				if err := appCore.StageFiles(ctx, ".", []string{"-A"}); err != nil {
					return err
				}
				cs, err = appCore.GetCommitChanges(ctx, ".")
				if err != nil {
					return err
				}
			}
			if cs.StagedCount() == 0 {
				if manual || all {
					return fmt.Errorf("no staged files; use --all or git add before --message")
				}
				fmt.Fprintln(out, warnStyle.Render("Nenhum arquivo em stage. Deseja adicionar alterações?"))
				fmt.Fprintln(out, "[1] Adicionar todos os arquivos modificados (git add -A)\n[2] Selecionar arquivos individualmente\n[3] Cancelar")
				choice, err := readCommitChoice(reader, out, "Escolha [1/2/3]: ", []string{"1", "2", "3"})
				if err != nil {
					return err
				}
				if choice == "3" {
					fmt.Fprintln(out, subtleStyle.Render("Commit cancelado pelo usuário."))
					return nil
				}
				paths := []string{"-A"}
				if choice == "2" {
					paths, err = selectCommitFiles(reader, out, cs.Files)
					if err != nil {
						return err
					}
				}
				if err := appCore.StageFiles(ctx, ".", paths); err != nil {
					return err
				}
			}
			report, err := appCore.CheckCommit(ctx, ".")
			if err != nil {
				return fmt.Errorf("safety check failed; nothing committed: %w", err)
			}
			override, err := guardGate(out, reader, report, "commit", allowSecrets, yes)
			if err != nil {
				return err
			}
			if !manual {
				if commitType == "" {
					types := []string{"feat", "fix", "chore", "refactor", "docs", "test", "style", "perf"}
					fmt.Fprintln(out, titleStyle.Render("Tipo de Commit:"))
					for i, value := range types {
						fmt.Fprintf(out, "[%d] %s\n", i+1, branchStyle.Render(value))
					}
					choice, err := readCommitChoice(reader, out, "Escolha [1-8]: ", []string{"1", "2", "3", "4", "5", "6", "7", "8"})
					if err != nil {
						return err
					}
					index, _ := strconv.Atoi(choice)
					commitType = types[index-1]
				}
				proposal, err := appCore.ProposeCommit(ctx, ".", commitType)
				if err != nil {
					return err
				}
				message = proposal.Message()
				fmt.Fprintln(out, titleStyle.Render("Mensagem sugerida: ")+branchStyle.Render(message))
				choice, err := readCommitChoice(reader, out, "[Y/e/n] Confirmar commit? (Y=confirmar, e=editar mensagem, n=cancelar): ", []string{"", "y", "e", "n"})
				if err != nil {
					return err
				}
				if choice == "n" {
					fmt.Fprintln(out, warnStyle.Render("Commit cancelado pelo usuário. Os arquivos permanecem em stage."))
					return nil
				}
				if choice == "e" {
					for {
						fmt.Fprint(out, "Nova mensagem: ")
						message, err = readCommitLine(reader)
						if err != nil {
							return err
						}
						if strings.TrimSpace(message) != "" {
							break
						}
						fmt.Fprintln(out, errorStyle.Render("A mensagem não pode estar vazia."))
					}
				}
			}
			result, err := appCore.CreateCommit(ctx, ".", message, override)
			if err != nil {
				printBlocked(out, err)
				return err
			}
			fmt.Fprintf(out, "%s %s %s\n", successStyle.Render("✓ Commit criado:"), branchStyle.Render(result.ShortHash), result.Subject)
			if n := len(result.Overridden); n > 0 {
				fmt.Fprintln(out, warnStyle.Render(fmt.Sprintf("⚠ Override registrado: commit criado com %d achado(s) bloqueante(s) confirmados.", n)))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Adicionar todos os arquivos modificados/untracked (git add -A)")
	cmd.Flags().StringVarP(&commitType, "type", "t", "", "Tipo: feat, fix, chore, refactor, docs, test, style, perf (mantém confirmação)")
	cmd.Flags().StringVarP(&message, "message", "m", "", "Mensagem explícita; comita sem confirmação (requer stage ou --all)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Prever arquivos e mensagem sem alterar stage/histórico (tipo padrão: chore)")
	cmd.Flags().BoolVar(&allowSecrets, "allow-secrets", false, "Permitir commit com achados bloqueantes após digitar CONFIRMAR")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Com --allow-secrets: confirmar o override sem digitar (scripts)")
	return cmd
}

func readCommitLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		// EOF is not an empty confirmation. Never authorize a commit implicitly.
		return "", fmt.Errorf("interactive input interrupted; no commit created (use --message in scripts): %w", err)
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

func readCommitChoice(reader *bufio.Reader, out io.Writer, prompt string, choices []string) (string, error) {
	for {
		fmt.Fprint(out, prompt)
		line, err := readCommitLine(reader)
		if err != nil {
			return "", err
		}
		line = strings.ToLower(strings.TrimSpace(line))
		for _, choice := range choices {
			if line == choice {
				return line, nil
			}
		}
		fmt.Fprintln(out, errorStyle.Render("Opção inválida. Tente novamente."))
	}
}

func selectCommitFiles(reader *bufio.Reader, out io.Writer, files []domain.FileStatus) ([]string, error) {
	for i, file := range files {
		fmt.Fprintf(out, "[%d] %q\n", i+1, file.Path)
	}
	for {
		fmt.Fprint(out, "Arquivos (ex: 1,3): ")
		line, err := readCommitLine(reader)
		if err != nil {
			return nil, err
		}
		var paths []string
		seen := map[int]bool{}
		valid := true
		for _, token := range strings.Split(line, ",") {
			index, err := strconv.Atoi(strings.TrimSpace(token))
			if err != nil || index < 1 || index > len(files) {
				valid = false
				break
			}
			if !seen[index] {
				// Prefix literal selections so a file named -A cannot mean stage-all.
				paths = append(paths, "./"+files[index-1].Path)
				seen[index] = true
			}
		}
		if valid && len(paths) > 0 {
			return paths, nil
		}
		fmt.Fprintln(out, errorStyle.Render("Seleção inválida. Informe índices separados por vírgula."))
	}
}

func newPushCmd(appCore app.PushService) *cobra.Command {
	var yes, allowSecrets bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Explicitly push local commits to remote target",
		Long: "Publica somente a branch local no destino exibido, sem force.\n" +
			"Com upstream, usa o remote e a branch de destino configurados.\n" +
			"Sem upstream, propõe origin (ou o único remote) e configura upstream\n" +
			"após confirmação. Múltiplos remotes sem origin exigem upstream explícito.\n" +
			"O destino e HEAD são revalidados antes de enviar. Nenhum fetch automático.\n" +
			"Antes de enviar, avalia cada commit que ainda não existe em nenhum remote:\n" +
			"segredo ou marcador de conflito bloqueia e o vorq mostra como corrigir\n" +
			"(nunca reescreve o histórico sozinho). --allow-secrets permite seguir\n" +
			"após digitar CONFIRMAR; em scripts, --allow-secrets --yes.",
		Example: "  vorq push\n  vorq push --yes\n  vorq push --allow-secrets",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := appCore.GetPushTarget(cmd.Context(), ".")
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Local: %s -> Remote: %s/%s\n", branchStyle.Render(target.Branch), branchStyle.Render(target.Remote), branchStyle.Render(target.RemoteBranch))
			prompt := fmt.Sprintf("Publicar commits em %s/%s? [Y/n]: ", target.Remote, target.RemoteBranch)
			if target.HasUpstream && target.Ahead >= 0 {
				prompt = fmt.Sprintf("Publicar %d commits em %s/%s? [Y/n]: ", target.Ahead, target.Remote, target.RemoteBranch)
			}
			if !target.HasUpstream {
				prompt = "Branch sem upstream. Deseja publicar e configurar upstream? [Y/n]: "
			}
			reader := bufio.NewReader(cmd.InOrStdin())
			if target.Guard != nil {
				fmt.Fprintln(out, subtleStyle.Render(fmt.Sprintf("Verificação de segurança: %d commit(s) não publicados avaliados.", target.Guard.Commits)))
			}
			override, err := guardGate(out, reader, target.Guard, "push", allowSecrets, yes)
			if err != nil {
				return err
			}
			if !yes && !override {
				for {
					fmt.Fprint(out, prompt)
					line, err := reader.ReadString('\n')
					if err != nil {
						return fmt.Errorf("push confirmation interrupted; nothing published: %w", err)
					}
					choice := strings.ToLower(strings.TrimSpace(line))
					if choice == "n" {
						fmt.Fprintln(out, subtleStyle.Render("Push cancelado. Nada publicado."))
						return nil
					}
					if choice == "" || choice == "y" {
						break
					}
					fmt.Fprintln(out, warnStyle.Render("Opção inválida. Use Y ou n."))
				}
			}
			result, err := appCore.PushChanges(cmd.Context(), ".", domain.PushOptions{Target: *target, AllowFindings: override})
			if err != nil {
				printBlocked(out, err)
				return err
			}
			fmt.Fprintln(out, successStyle.Render("✓ Push concluído: ")+target.Remote+"/"+target.RemoteBranch)
			if n := len(result.Overridden); n > 0 {
				fmt.Fprintln(out, warnStyle.Render(fmt.Sprintf("⚠ Override registrado: push com %d achado(s) bloqueante(s) confirmados.", n)))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Publicar no destino exibido sem confirmação interativa")
	cmd.Flags().BoolVar(&allowSecrets, "allow-secrets", false, "Permitir push com achados bloqueantes após digitar CONFIRMAR (scripts: --yes)")
	return cmd
}

func newRmCmd(appCore app.WorkspaceService) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "rm <worktree-name>",
		Aliases: []string{"remove"},
		Short:   "Safely teardown a worktree checking for uncommitted and ignored files",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			target := args[0]
			ctx := cmd.Context()

			report, err := appCore.RemoveWorkspace(ctx, ".", target, force)
			if err != nil {
				fmt.Fprintln(out)
				fmt.Fprintln(out, errorStyle.Render("🛑 Worktree Removal Blocked"))
				fmt.Fprintf(out, "Safety guards prevented deletion of workspace %s:\n\n", titleStyle.Render(target))

				if report != nil && len(report.Violations) > 0 {
					canForce := true
					for _, v := range report.Violations {
						fmt.Fprintf(out, "  %s %s\n", warnStyle.Render("•"), v.Description)
						if v.Type == domain.ViolationMainWorkspace || v.Type == domain.ViolationActiveWorkspace {
							canForce = false
						}
					}
					fmt.Fprintln(out)
					if canForce {
						fmt.Fprintln(out, subtleStyle.Render("Hint: To permanently discard uncommitted/ignored files, run:"))
						fmt.Fprintf(out, "  vorq rm %s --force\n\n", target)
					}
				} else {
					fmt.Fprintf(out, "  %s %s\n\n", warnStyle.Render("•"), err.Error())
				}
				return err
			}

			fmt.Fprintln(out)
			fmt.Fprintf(out, "%s Worktree removed successfully!\n", successStyle.Render("✓"))
			fmt.Fprintf(out, "  Target: %s\n", subtleStyle.Render(target))
			fmt.Fprintf(out, "  %s Branch remains preserved in Git.\n\n", subtleStyle.Render("ℹ Note:"))
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force teardown bypassing uncommitted and ignored files check")
	return cmd
}

func newPruneCmd(appCore app.WorkspaceService) *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Prune linked worktrees information from Git database",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			ctx := cmd.Context()
			pruned, err := appCore.PruneWorkspaces(ctx, ".")
			if err != nil {
				return err
			}
			fmt.Fprintln(out)
			fmt.Fprintf(out, "%s Worktree administrative references pruned.\n", successStyle.Render("✓"))
			if strings.TrimSpace(pruned) != "" {
				fmt.Fprintln(out, subtleStyle.Render(strings.TrimSpace(pruned)))
			}
			fmt.Fprintln(out)
			return nil
		},
	}
}
