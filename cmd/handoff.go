package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/antopolskiy/kanban-md/internal/clierr"
	"github.com/antopolskiy/kanban-md/internal/config"
	"github.com/antopolskiy/kanban-md/internal/output"
	"github.com/antopolskiy/kanban-md/internal/store"
	"github.com/antopolskiy/kanban-md/internal/task"
)

var handoffCmd = &cobra.Command{
	Use:   "handoff ID",
	Short: "Hand off a task (move to review with notes)",
	Long: `Moves a task to review status, appends a handoff note, and optionally
blocks the task and/or releases the claim. Designed for multi-agent workflows
where standardized handoffs prevent information loss.`,
	Args: cobra.ExactArgs(1),
	RunE: runHandoff,
}

func init() {
	handoffCmd.Flags().String("claim", "", "claim task for an agent (required)")
	handoffCmd.Flags().String("note", "", "handoff note to append to body")
	handoffCmd.Flags().BoolP("timestamp", "t", false, "prefix a timestamp line to the note")
	handoffCmd.Flags().String("block", "", "mark task as blocked with reason")
	handoffCmd.Flags().Bool("release", false, "release claim after handoff")
	rootCmd.AddCommand(handoffCmd)
}

func runHandoff(cmd *cobra.Command, args []string) error {
	ids, err := parseIDs(args[0])
	if err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if len(ids) == 1 {
		return handoffSingleTask(cfg, ids[0], cmd)
	}

	return runBatch(ids, func(id int) error {
		_, err := executeHandoff(cfg, id, cmd)
		return err
	})
}

func handoffSingleTask(cfg *config.Config, id int, cmd *cobra.Command) error {
	t, err := executeHandoff(cfg, id, cmd)
	if err != nil {
		return err
	}

	if outputFormat() == output.FormatJSON {
		return output.JSON(os.Stdout, t)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Handed off task #%d -> review", t.ID))
	if t.Blocked {
		parts = append(parts, fmt.Sprintf("(blocked: %s)", t.BlockReason))
	}
	if t.ClaimedBy == "" {
		parts = append(parts, "(claim released)")
	}
	output.Messagef(os.Stdout, "%s", strings.Join(parts, " "))
	return nil
}

func executeHandoff(cfg *config.Config, id int, cmd *cobra.Command) (*task.Task, error) {
	claimant, _ := cmd.Flags().GetString("claim")
	release, _ := cmd.Flags().GetBool("release")
	blockReason, _ := cmd.Flags().GetString("block")
	note, _ := cmd.Flags().GetString("note")
	addTimestamp, _ := cmd.Flags().GetBool("timestamp")

	if claimant == "" {
		return nil, clierr.New(clierr.InvalidInput, "claim name is required (use --claim NAME)")
	}
	if cfg.UsesRefStorage() {
		return executeHandoffRef(cfg, id, cmd, claimant, release, blockReason, note, addTimestamp)
	}
	return executeHandoffFile(cfg, id, cmd, claimant, release, blockReason, note, addTimestamp)
}

func executeHandoffFile(cfg *config.Config, id int, cmd *cobra.Command, claimant string, release bool, blockReason, note string, addTimestamp bool) (*task.Task, error) {
	path, err := task.FindByID(cfg.TasksPath(), id)
	if err != nil {
		return nil, err
	}

	t, err := task.Read(path)
	if err != nil {
		return nil, err
	}

	// Validate claim ownership.
	if err = checkClaim(t, claimant, cfg.ClaimTimeoutDuration()); err != nil {
		return nil, err
	}

	// Resolve target status: "review" must exist in config.
	const reviewStatus = "review"
	if err = task.ValidateStatus(reviewStatus, cfg.StatusNames()); err != nil {
		return nil, clierr.New(clierr.InvalidInput,
			"board has no 'review' status; add one to use handoff")
	}

	// Move to review (skip if already there).
	oldStatus := t.Status
	if t.Status != reviewStatus {
		// Enforce require_claim for review.
		if cfg.StatusRequiresClaim(reviewStatus) && claimant == "" {
			return nil, task.ValidateClaimRequired(reviewStatus)
		}
		if err = enforceMoveWIP(cfg, t, reviewStatus); err != nil {
			return nil, err
		}
		t.Status = reviewStatus
		task.UpdateTimestamps(t, oldStatus, reviewStatus, cfg)
	}

	// Apply claim (refresh).
	now := time.Now()
	t.ClaimedBy = claimant
	t.ClaimedAt = &now

	// Optionally block.
	if cmd.Flags().Changed("block") {
		if blockReason == "" {
			return nil, clierr.New(clierr.InvalidInput, "block reason is required (use --block REASON)")
		}
		t.Blocked = true
		t.BlockReason = blockReason
	}

	// Append note.
	if note != "" {
		t.Body = appendBody(t.Body, note, addTimestamp)
	}

	// Release claim if requested.
	if release {
		t.ClaimedBy = ""
		t.ClaimedAt = nil
	}

	t.Updated = time.Now()

	if err = task.Write(path, t); err != nil {
		return nil, fmt.Errorf("writing task: %w", err)
	}

	// Log activity.
	logHandoffActivity(cfg, t, oldStatus)
	return t, nil
}

func executeHandoffRef(cfg *config.Config, id int, cmd *cobra.Command, claimant string, release bool, blockReason, note string, addTimestamp bool) (*task.Task, error) {
	st, err := newStore(cfg)
	if err != nil {
		return nil, err
	}

	var handedOff *task.Task
	oldStatus := ""
	if _, err := st.Mutate(context.Background(), func(snap *store.Snapshot) error {
		t, findErr := findTaskInSnapshot(snap.Tasks, id)
		if findErr != nil {
			return findErr
		}
		if claimErr := checkClaim(t, claimant, cfg.ClaimTimeoutDuration()); claimErr != nil {
			return claimErr
		}
		const reviewStatus = "review"
		if statusErr := task.ValidateStatus(reviewStatus, cfg.StatusNames()); statusErr != nil {
			return clierr.New(clierr.InvalidInput,
				"board has no 'review' status; add one to use handoff")
		}
		oldStatus = t.Status
		if t.Status != reviewStatus {
			if cfg.StatusRequiresClaim(reviewStatus) && claimant == "" {
				return task.ValidateClaimRequired(reviewStatus)
			}
			if wipErr := enforceSnapshotWIPLimit(cfg, snap.Tasks, t, t.Status, reviewStatus); wipErr != nil {
				return wipErr
			}
			t.Status = reviewStatus
			task.UpdateTimestamps(t, oldStatus, reviewStatus, cfg)
		}

		now := time.Now()
		t.ClaimedBy = claimant
		t.ClaimedAt = &now
		if cmd.Flags().Changed("block") {
			if blockReason == "" {
				return clierr.New(clierr.InvalidInput, "block reason is required (use --block REASON)")
			}
			t.Blocked = true
			t.BlockReason = blockReason
		}
		if note != "" {
			t.Body = appendBody(t.Body, note, addTimestamp)
		}
		if release {
			t.ClaimedBy = ""
			t.ClaimedAt = nil
		}
		t.Updated = time.Now()
		handedOff = t
		return nil
	}); err != nil {
		return nil, err
	}

	logHandoffActivity(cfg, handedOff, oldStatus)
	return handedOff, nil
}

func logHandoffActivity(cfg *config.Config, t *task.Task, oldStatus string) {
	if oldStatus != t.Status {
		logActivity(cfg, "move", t.ID, oldStatus+" -> "+t.Status)
	}
	logActivity(cfg, "handoff", t.ID, t.Title)
	if t.Blocked {
		logActivity(cfg, "block", t.ID, t.BlockReason)
	}
	if t.ClaimedBy == "" {
		logActivity(cfg, "release", t.ID, t.Title)
	}
}
