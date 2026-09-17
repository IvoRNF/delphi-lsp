package lsp

import (
	"strings"
	"testing"
)

func TestDeclarationSourcePreservesPositionsAndConditionalState(t *testing.T) {
	source := "uses Classes{$IFDEF V},Variants{$ELSE},Other{$ENDIF};\r\n" +
		"{$IFDEF A}First{$ELSE}{$IFDEF B}Hidden{$ENDIF}AlsoHidden{$ENDIF}Last\n" +
		"const Text = '{$IFDEF Literal}'; // {$IFDEF Comment}\n" +
		"(*$IFNDEF V*)Visible(*$ELSE*)Invisible(*$ENDIF*);"
	got, depth := declarationSource(source)
	if depth != 1 || len(got) != len(source) {
		t.Fatalf("length/depth changed: %d/%d, depth %d", len(got), len(source), depth)
	}
	for _, kept := range []string{"Classes", ",Variants", "First", "Last", "'{$IFDEF Literal}'", "Visible"} {
		offset := strings.Index(source, kept)
		if got[offset:offset+len(kept)] != kept {
			t.Errorf("lost or moved %q in %q", kept, got)
		}
	}
	for _, removed := range []string{",Other", "Hidden", "AlsoHidden", "Invisible", "// {$IFDEF Comment}"} {
		if strings.Contains(got, removed) {
			t.Errorf("retained inactive code/comment %q", removed)
		}
	}
	for i := range source {
		if strings.ContainsRune("\r\n;", rune(source[i])) && got[i] != source[i] {
			t.Errorf("lost separator at %d", i)
		}
	}
}
