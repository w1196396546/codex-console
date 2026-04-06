package adminui

import (
	"fmt"
	"path/filepath"
	"runtime"
)

type AssetPaths struct {
	BackendRoot  string
	TemplatesDir string
	StaticDir    string
}

func resolveAssetPaths(paths AssetPaths) (AssetPaths, error) {
	if stringsTrimSpace(paths.BackendRoot) == "" {
		root, err := defaultBackendRoot()
		if err != nil {
			return AssetPaths{}, err
		}
		paths.BackendRoot = root
	}
	if stringsTrimSpace(paths.TemplatesDir) == "" {
		paths.TemplatesDir = filepath.Join(paths.BackendRoot, "web", "templates")
	}
	if stringsTrimSpace(paths.StaticDir) == "" {
		paths.StaticDir = filepath.Join(paths.BackendRoot, "web", "static")
	}
	return paths, nil
}

func defaultBackendRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("locate adminui source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}
