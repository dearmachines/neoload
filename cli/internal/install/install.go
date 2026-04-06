package install

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"neoload/internal/targets"
)

// Install copies skillFS into each target's skill subdirectory.
// Returns installed paths, skipped paths (already exist without force),
// total file count, and any error from real I/O failures.
// Skipped targets are not errors — they represent targets where the skill
// is already present. The caller can merge them with installed paths for
// lock file tracking.
func Install(skillFS fs.FS, targetList []targets.Target, skill string, force bool) (installed []string, skipped []string, fileCount int, err error) {
	var errs []error

	for _, t := range targetList {
		destDir := filepath.Join(t.SkillsDir, skill)

		if _, statErr := os.Stat(destDir); statErr == nil && !force {
			skipped = append(skipped, destDir)
			continue
		}

		n, installErr := installToTarget(skillFS, destDir)
		if installErr != nil {
			errs = append(errs, fmt.Errorf("install to %s: %w", destDir, installErr))
			continue
		}

		installed = append(installed, destDir)
		fileCount += n
	}

	return installed, skipped, fileCount, errors.Join(errs...)
}

// installToTarget atomically installs skillFS into destDir by writing to a
// temp directory first and then renaming it into place.
func installToTarget(skillFS fs.FS, destDir string) (int, error) {
	parent := filepath.Dir(destDir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return 0, fmt.Errorf("create parent directory: %w", err)
	}

	tmpDir, err := os.MkdirTemp(parent, ".neoload-tmp-*")
	if err != nil {
		return 0, fmt.Errorf("create temp directory: %w", err)
	}

	success := false
	defer func() {
		if !success {
			os.RemoveAll(tmpDir)
		}
	}()

	count, err := copyFS(skillFS, tmpDir)
	if err != nil {
		return 0, err
	}

	// Remove existing destination before rename.
	if _, statErr := os.Stat(destDir); statErr == nil {
		if err := os.RemoveAll(destDir); err != nil {
			return 0, fmt.Errorf("remove existing installation: %w", err)
		}
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		return 0, fmt.Errorf("finalize installation: %w", err)
	}

	success = true
	return count, nil
}

// copyFS copies all files from src into destDir, preserving directory structure.
func copyFS(src fs.FS, destDir string) (int, error) {
	count := 0
	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, path)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}

		f, err := src.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close()

		out, err := os.Create(dest)
		if err != nil {
			return fmt.Errorf("create %s: %w", dest, err)
		}
		defer out.Close()

		if _, err := io.Copy(out, f); err != nil {
			return fmt.Errorf("copy %s: %w", path, err)
		}

		count++
		return nil
	})
	return count, err
}
