package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/antopolskiy/kanban-md/internal/clierr"
	"github.com/antopolskiy/kanban-md/internal/config"
	"github.com/antopolskiy/kanban-md/internal/gitref"
	"github.com/antopolskiy/kanban-md/internal/output"
	"github.com/antopolskiy/kanban-md/internal/store"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new kanban board",
	Long:  `Creates a kanban directory with config.yml and initializes the board Git ref.`,
	RunE:  runInit,
}

func init() {
	initCmd.Flags().String("name", "", "board name (defaults to current directory name)")
	initCmd.Flags().StringSlice("statuses", nil, "comma-separated list of statuses")
	initCmd.Flags().StringSlice("wip-limit", nil, "WIP limit per status (format: status:N, repeatable)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, _ []string) error {
	dir := flagDir
	if dir == "" {
		dir = config.DefaultDir
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Check if already initialized.
	if _, statErr := os.Stat(filepath.Join(absDir, config.ConfigFileName)); statErr == nil {
		return clierr.Newf(clierr.BoardAlreadyExists, "board already initialized in %s", absDir).
			WithDetails(map[string]any{"dir": absDir})
	}

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		cwd, getwdErr := os.Getwd()
		if getwdErr != nil {
			return fmt.Errorf("getting working directory: %w", getwdErr)
		}
		name = filepath.Base(cwd)
	}

	cfg := config.NewDefault(name)
	cfg.SetDir(absDir)
	cfg.TasksDir = ""
	cfg.NextID = 0

	if applyErr := applyInitFlags(cmd, cfg); applyErr != nil {
		return applyErr
	}

	// Create directories.
	const dirMode = 0o750
	if mkdirErr := os.MkdirAll(absDir, dirMode); mkdirErr != nil {
		return fmt.Errorf("creating kanban directory: %w", mkdirErr)
	}

	// Write config.
	if saveErr := cfg.Save(); saveErr != nil {
		return fmt.Errorf("writing config: %w", saveErr)
	}

	notificationMode, hookWarning, err := configureNotifications(context.Background(), cfg)
	if err != nil {
		return err
	}
	cfg.Storage.Notifications.Mode = notificationMode
	if saveErr := cfg.Save(); saveErr != nil {
		return fmt.Errorf("writing config: %w", saveErr)
	}

	st, err := store.NewGitStore(context.Background(), cfg)
	if err != nil {
		return err
	}
	if _, err := st.Initialize(context.Background()); err != nil {
		return fmt.Errorf("initializing board ref: %w", err)
	}

	// Output result.
	return outputInitResult(cfg, absDir, name, hookWarning)
}

func outputInitResult(cfg *config.Config, absDir, name, hookWarning string) error {
	if hookWarning != "" {
		output.Messagef(os.Stderr, "Warning: %s", hookWarning)
	}
	format := outputFormat()
	if format == output.FormatJSON {
		return output.JSON(os.Stdout, map[string]string{
			"status":        "initialized",
			"dir":           absDir,
			"name":          name,
			"config":        cfg.ConfigPath(),
			"storage":       cfg.Storage.Ref,
			"notifications": cfg.Storage.Notifications.Mode,
			"columns":       strings.Join(cfg.StatusNames(), ","),
		})
	}

	output.Messagef(os.Stdout, "Initialized board %q in %s", name, absDir)
	output.Messagef(os.Stdout, "  Config:  %s", cfg.ConfigPath())
	output.Messagef(os.Stdout, "  Storage: %s", cfg.Storage.Ref)
	output.Messagef(os.Stdout, "  Notifications: %s", cfg.Storage.Notifications.Mode)
	output.Messagef(os.Stdout, "  Columns: %s", strings.Join(cfg.StatusNames(), ", "))
	output.Messagef(os.Stdout, "  Hint:    Install agent skills with: kanban-md skill install")

	if err := offerAddKanbanToGitignore(absDir); err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}

	return nil
}

func configureNotifications(ctx context.Context, cfg *config.Config) (string, string, error) {
	if cfg.Storage.Notifications.Mode == config.NotificationModePoll {
		return config.NotificationModePoll, "", nil
	}
	repo, err := gitref.Open(ctx, cfg.Dir())
	if err != nil {
		return "", "", err
	}
	installed, path, err := repo.InstallReferenceTransactionHook(ctx)
	if err != nil {
		return "", "", err
	}
	if installed {
		return config.NotificationModeHook, "", nil
	}
	return config.NotificationModePoll, "existing reference-transaction hook left unchanged at " + path + "; using polling notifications", nil
}

func applyInitFlags(cmd *cobra.Command, cfg *config.Config) error {
	if statuses, _ := cmd.Flags().GetStringSlice("statuses"); len(statuses) > 0 {
		sc := make([]config.StatusConfig, len(statuses))
		for i, s := range statuses {
			sc[i] = config.StatusConfig{Name: s}
		}
		cfg.Statuses = sc
		cfg.Defaults.Status = statuses[0]
	}

	if wipLimits, _ := cmd.Flags().GetStringSlice("wip-limit"); len(wipLimits) > 0 {
		parsed, err := parseWIPLimits(wipLimits)
		if err != nil {
			return err
		}
		cfg.WIPLimits = parsed
	}

	return cfg.Validate()
}

// parseWIPLimits parses "status:N" pairs into a map.
func parseWIPLimits(pairs []string) (map[string]int, error) {
	limits := make(map[string]int, len(pairs))
	for _, pair := range pairs {
		parts := strings.SplitN(pair, ":", 2) //nolint:mnd // key:value pair
		if len(parts) != 2 {                  //nolint:mnd // key:value pair
			return nil, fmt.Errorf("invalid WIP limit %q (expected status:N)", pair)
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid WIP limit value %q in %q", parts[1], pair)
		}
		limits[parts[0]] = n
	}
	return limits, nil
}
