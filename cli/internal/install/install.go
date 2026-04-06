package install

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"neoload/internal/targets"
)

// Install copies skillFS into each target's skill subdirectory.
// Returns the list of installed paths, total file count, and any error.
// If force is false and a destination already exists, it returns an error.
func Install(skillFS fs.FS, targetList []targets.Target, skill string, force bool) ([]string, int, error) {
	var installedPaths []string
	var totalFiles int

	for _, t := range targetList {
		destDir := filepath.Join(t.SkillsDir, skill)

		if _, err := os.Stat(destDir); err == nil && !force {
			return nil, 0, fmt.Errorf("skill already installed at %s; use --force to overwrite", destDir)
		}

		n, err := installToTarget(skillFS, destDir)
		if err != nil {
			return nil, 0, fmt.Errorf("install to %s: %w", destDir, err)
		}

		installedPaths = append(installedPaths, destDir)
		totalFiles += n
	}

	return installedPaths, totalFiles, nil
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
