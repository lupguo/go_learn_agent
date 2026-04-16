package s02_tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePath resolves a path relative to workDir and ensures it doesn't escape.
func SafePath(workDir, path string) (string, error) {
	var abs string
	if filepath.IsAbs(path) {
		abs = filepath.Clean(path)
	} else {
		abs = filepath.Clean(filepath.Join(workDir, path))
	}
	workDir = filepath.Clean(workDir)
	if !strings.HasPrefix(abs, workDir+string(filepath.Separator)) && abs != workDir {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}
	return abs, nil
}
