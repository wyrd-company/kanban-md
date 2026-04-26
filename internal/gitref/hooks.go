package gitref

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const referenceTransactionHook = "reference-transaction"

// ReferenceTransactionHookPath returns the repository hook path for ref updates.
func (r *Repository) ReferenceTransactionHookPath(ctx context.Context) (string, error) {
	hooksPath, err := r.configuredHooksPath(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(hooksPath, referenceTransactionHook), nil
}

// InstallReferenceTransactionHook installs the tiny kanban-md hook shim when no
// hook is already present. It returns false when an existing hook was left alone.
func (r *Repository) InstallReferenceTransactionHook(ctx context.Context) (bool, string, error) {
	path, err := r.ReferenceTransactionHookPath(ctx)
	if err != nil {
		return false, "", err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return false, path, nil
	} else if !os.IsNotExist(statErr) {
		return false, path, fmt.Errorf("inspecting hook %s: %w", path, statErr)
	}

	const dirMode = 0o750
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return false, path, fmt.Errorf("creating hook directory: %w", err)
	}
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"committed\" ]; then\n" +
		"  command -v kanban-md >/dev/null 2>&1 || exit 0\n" +
		"  kanban-md hook reference-transaction \"$@\"\n" +
		"fi\n"
	const fileMode = 0o755
	if err := os.WriteFile(path, []byte(body), fileMode); err != nil {
		return false, path, fmt.Errorf("writing hook %s: %w", path, err)
	}
	return true, path, nil
}

func (r *Repository) configuredHooksPath(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "config", "--path", "--get", "core.hooksPath")
	if err == nil {
		path := strings.TrimSpace(string(out))
		if path != "" {
			if filepath.IsAbs(path) {
				return path, nil
			}
			return filepath.Join(r.WorkDir, path), nil
		}
	}

	out, err = r.run(ctx, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("locating git common dir: %w", err)
	}
	commonDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(r.WorkDir, commonDir)
	}
	return filepath.Join(commonDir, "hooks"), nil
}
