package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigIncludesProjectUnitSearchPaths(t *testing.T) {
	workspace := t.TempDir()
	projectDir := filepath.Join(workspace, "project")
	sharedDir := filepath.Join(workspace, "shared")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(projectDir, "Main.dproj")
	if err := os.WriteFile(project, []byte(`<?xml version="1.0"?><Project><PropertyGroup><DCC_UnitSearchPath>../shared;$(BDS)\lib</DCC_UnitSearchPath></PropertyGroup></Project>`), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(workspace, "delphi-lsp.json")
	if err := os.WriteFile(config, []byte(`{"project":"project/Main.dproj"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, roots, err := LoadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(sharedDir)
	for _, root := range roots {
		if stringsEqualFold(root, want) {
			return
		}
	}
	t.Fatalf("project unit search path %q missing from %#v", want, roots)
}
