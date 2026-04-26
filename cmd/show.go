package cmd

import (
	"context"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/antopolskiy/kanban-md/internal/clierr"
	"github.com/antopolskiy/kanban-md/internal/output"
	"github.com/antopolskiy/kanban-md/internal/store"
	"github.com/antopolskiy/kanban-md/internal/task"
)

var showCmd = &cobra.Command{
	Use:   "show ID",
	Short: "Show task details",
	Long:  `Displays full details of a single task including its markdown body.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	rootCmd.AddCommand(showCmd)
}

func runShow(_ *cobra.Command, args []string) error {
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return task.ValidateTaskID(args[0])
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if cfg.UsesRefStorage() {
		st, storeErr := store.NewGitStore(context.Background(), cfg)
		if storeErr != nil {
			return storeErr
		}
		snap, loadErr := st.Load(context.Background())
		if loadErr != nil {
			return loadErr
		}
		for _, t := range snap.Tasks {
			if t.ID == id {
				return outputTaskDetail(t)
			}
		}
		return clierr.Newf(clierr.TaskNotFound, "task not found: #%d", id).
			WithDetails(map[string]any{"id": id})
	}

	path, err := task.FindByID(cfg.TasksPath(), id)
	if err != nil {
		return err
	}

	t, err := task.Read(path)
	if err != nil {
		return err
	}

	return outputTaskDetail(t)
}

func outputTaskDetail(t *task.Task) error {
	format := outputFormat()
	if format == output.FormatJSON {
		return output.JSON(os.Stdout, t)
	}
	if format == output.FormatCompact {
		output.TaskDetailCompact(os.Stdout, t)
		return nil
	}

	output.TaskDetail(os.Stdout, t)
	return nil
}
