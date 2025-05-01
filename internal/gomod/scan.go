package gomod

import (
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// Scan は root 以下の全ての go.mod を解析し、ModuleMapを返します
func Scan(root string) (*ModuleMap, error) {
	m := map[string]Module{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "go.mod" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mf, err := modfile.Parse(path, data, nil)
		if err != nil {
			return err
		}
		modPath := mf.Module.Mod.Path
		reqs := make([]string, 0, len(mf.Require))
		for _, r := range mf.Require {
			reqs = append(reqs, r.Mod.Path)
		}
		m[modPath] = Module{Path: modPath, Requires: reqs, Root: filepath.Dir(path)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return NewModuleMap(m), nil
}
