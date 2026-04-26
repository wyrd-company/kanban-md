package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const notifyFileName = ".notify"

const (
	referenceTransactionFieldCount = 3
	notifyFileMode                 = 0o600
)

var hookCmd = &cobra.Command{
	Use:    "hook",
	Short:  "Internal Git hook entrypoints",
	Hidden: true,
}

var hookReferenceTransactionCmd = &cobra.Command{
	Use:   "reference-transaction STATE",
	Short: "Handle Git reference-transaction hook events",
	Args:  cobra.ExactArgs(1),
	RunE:  runHookReferenceTransaction,
}

type notifyPayload struct {
	Ref string `json:"ref"`
	Rev string `json:"rev"`
	At  string `json:"at"`
}

func init() {
	hookCmd.AddCommand(hookReferenceTransactionCmd)
	rootCmd.AddCommand(hookCmd)
}

func runHookReferenceTransaction(_ *cobra.Command, args []string) error {
	if args[0] != "committed" {
		return nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	rev := ""
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != referenceTransactionFieldCount { // old new ref
			continue
		}
		if fields[2] == cfg.Storage.Ref {
			rev = fields[1]
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return fmt.Errorf("reading reference transaction: %w", scanErr)
	}
	if rev == "" {
		return nil
	}

	payload := notifyPayload{
		Ref: cfg.Storage.Ref,
		Rev: rev,
		At:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling notification: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(cfg.Dir(), notifyFileName), data, notifyFileMode)
}
