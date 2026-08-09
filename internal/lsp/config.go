package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Project      string   `json:"project"`
	DelphiDir    string   `json:"delphiDir"`
	SearchPaths  []string `json:"searchPaths"`
	IncludePaths []string `json:"includePaths"`
}

func LoadConfig(name string) (Config, []string, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return Config{}, nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, nil, fmt.Errorf("invalid Delphi LSP config: %w", err)
	}
	base := filepath.Dir(name)
	resolve := func(p string) string {
		if p == "" {
			return ""
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		return filepath.Clean(p)
	}
	c.Project, c.DelphiDir = resolve(c.Project), resolve(c.DelphiDir)
	roots := []string{}
	if c.Project != "" {
		if stringsEqualFold(filepath.Ext(c.Project), ".dproj") {
			roots = append(roots, filepath.Dir(c.Project))
		} else {
			return c, nil, fmt.Errorf("project must be a .dproj file: %s", c.Project)
		}
	}
	if c.DelphiDir != "" {
		roots = append(roots, c.DelphiDir)
	}
	for _, p := range c.SearchPaths {
		roots = append(roots, resolve(p))
	}
	for _, p := range c.IncludePaths {
		roots = append(roots, resolve(p))
	}
	return c, uniqueExistingDirs(roots), nil
}

func stringsEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

func uniqueExistingDirs(paths []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		info, err := os.Stat(p)
		if err == nil && info.IsDir() {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
