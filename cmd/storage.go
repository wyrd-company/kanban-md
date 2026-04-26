package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/antopolskiy/kanban-md/internal/output"
	"github.com/antopolskiy/kanban-md/internal/store"
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Inspect board storage",
}

var storageStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Git-ref storage status",
	RunE:  runStorageStatus,
}

type storageStatus struct {
	Ref              string `json:"ref"`
	Revision         string `json:"revision,omitempty"`
	SnapshotVersion  int    `json:"snapshot_version,omitempty"`
	NextID           int    `json:"next_id,omitempty"`
	TaskCount        int    `json:"task_count,omitempty"`
	NotificationMode string `json:"notification_mode,omitempty"`
	Repository       string `json:"repository,omitempty"`
	Present          bool   `json:"present"`
}

func init() {
	storageCmd.AddCommand(storageStatusCmd)
	rootCmd.AddCommand(storageCmd)
}

func runStorageStatus(_ *cobra.Command, _ []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	st, err := store.NewGitStore(context.Background(), cfg)
	if err != nil {
		return err
	}

	status := storageStatus{
		Ref:              st.Ref(),
		NotificationMode: cfg.Storage.Notifications.Mode,
		Repository:       st.RepositoryPath(),
	}
	snap, err := st.Load(context.Background())
	if err != nil {
		if !errors.Is(err, store.ErrRefNotFound) {
			return err
		}
		return outputStorageStatus(status)
	}
	status.Present = true
	status.Revision = snap.Rev
	status.SnapshotVersion = 1
	status.NextID = snap.NextID
	status.TaskCount = len(snap.Tasks)
	return outputStorageStatus(status)
}

func outputStorageStatus(status storageStatus) error {
	if outputFormat() == output.FormatJSON {
		return output.JSON(os.Stdout, status)
	}
	if outputFormat() == output.FormatCompact {
		present := "missing"
		if status.Present {
			present = status.Revision
		}
		output.Messagef(os.Stdout, "%s %s next=%d tasks=%d mode=%s",
			status.Ref, present, status.NextID, status.TaskCount, status.NotificationMode)
		return nil
	}
	fmt.Fprintf(os.Stdout, "Storage: %s\n", status.Ref)
	fmt.Fprintf(os.Stdout, "Repository: %s\n", status.Repository)
	fmt.Fprintf(os.Stdout, "Notifications: %s\n", status.NotificationMode)
	if !status.Present {
		fmt.Fprintln(os.Stdout, "Revision: missing")
		return nil
	}
	fmt.Fprintf(os.Stdout, "Revision: %s\n", status.Revision)
	fmt.Fprintf(os.Stdout, "Snapshot version: %d\n", status.SnapshotVersion)
	fmt.Fprintf(os.Stdout, "Next ID: %d\n", status.NextID)
	fmt.Fprintf(os.Stdout, "Tasks: %d\n", status.TaskCount)
	return nil
}
