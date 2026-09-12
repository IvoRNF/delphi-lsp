package lsp

import (
	"strings"
	"testing"
)

func TestValidUnitStructure(t *testing.T) {
	for _, source := range []string{
		"unit U; interface implementation end.",
		"unit U; interface type EBarraProduto = class(Exception); implementation end.",
		"unit U; interface type E = class\n( { ancestor } SysUtils.Exception\n); R = record X: Integer end; implementation end.",
		"unit U; interface type T = class(TList<Integer>); implementation end.",
		"unit U; interface type T = class sealed(TObject); implementation end.",
		"unit U; interface type T = class(TObject) end; implementation end.",
		"\ufeffunit U; interface implementation end.",
		"unit U; interface implementation procedure A(Callback: procedure); begin end; begin A(nil) end.",
		"unit Company.U deprecated 'Use V'; interface implementation end.",
		"unit U; interface uses A, Company.B; implementation uses C in 'C.pas'; end.",
		"unit U; interface implementation initialization Run; finalization Stop; end.",
		"unit U; interface implementation begin Run end.",
		"unit U; interface type I = interface procedure Run; end; T = class; P = class of T; R = record case Boolean of True: (X: Integer); False: (Y: Integer) end; implementation end.",
		"unit U; interface type T = class(TObject) procedure Run; end; implementation procedure T.Run; begin try case X of 1: Run end finally repeat Run until Done end end; end.",
		"unit U; interface type T = procedure of object; R<T: record> = record X: T end; implementation end.",
		"unit U; interface implementation procedure A; forward; procedure B; external 'x'; end.",
		"unit U; interface implementation procedure A; procedure B; begin end; begin B end; end.",
		"unit U; interface implementation procedure A; asm mov eax, 1 end; end.",
		"{unit Fake;} unit U; interface implementation initialization Run('end. implementation'); end. // trailing comment",
		"unit U; interface {$IFDEF A} uses A; {$ELSE} uses B; {$ENDIF} implementation end.",
	} {
		if got := unitDiagnostics("file:///U.pas", source); len(got) != 0 {
			t.Errorf("%s\n%+v", source, got)
		}
	}
}

func TestInvalidUnitStructure(t *testing.T) {
	for _, tt := range []struct{ source, message string }{
		{"interface implementation end.", "Expected 'unit' heading"},
		{"unit ; interface implementation end.", "Expected unit identifier"},
		{"unit A.; interface implementation end.", "Expected identifier after '.'"},
		{"unit U interface implementation end.", "Expected ';' after unit heading"},
		{"unit U; implementation end.", "Missing 'interface' section"},
		{"unit U; interface end.", "Missing 'implementation' section"},
		{"unit U; interface interface implementation end.", "Duplicate 'interface' section"},
		{"unit U; interface implementation implementation end.", "Duplicate 'implementation' section"},
		{"unit U; interface implementation finalization end.", "requires an 'initialization'"},
		{"unit U; interface implementation initialization finalization initialization end.", "must precede 'finalization'"},
		{"unit U; interface var X: Integer; uses A; implementation end.", "'uses' must appear immediately"},
		{"unit U; interface uses A; uses B; implementation end.", "'uses' must appear immediately"},
		{"unit U; interface uses ; implementation end.", "Expected unit identifier"},
		{"unit U; interface uses A implementation end.", "Expected ';' after uses clause"},
		{"unit U; interface implementation end;", "Expected '.' after unit 'end'"},
		{"unit U; interface implementation", "Expected 'end.' to close unit"},
		{"unit U; interface implementation end. var X: Integer;", "Unexpected content after"},
		{"unit U; interface procedure A; begin end; implementation end.", "Routine bodies are not allowed"},
		{"unit U; interface type T = class implementation end.", "Unclosed block before"},
		{"unit U; interface type T = class(TObject) procedure Run; implementation end.", "Unclosed block before"},
		{"unit U; interface type T = class(Exception; implementation end.", "Unclosed block before"},
		{"unit U; interface implementation procedure A; begin end.", "Expected ';' after routine body"},
		{"unit U; interface implementation initialization begin", "Unclosed 'begin' block"},
	} {
		got := unitDiagnostics("file:///U.pas", tt.source)
		found := false
		for _, d := range got {
			if strings.Contains(d.Message, tt.message) {
				found = true
			}
			if d.Severity != 1 || d.Source != "delphi-lsp" {
				t.Errorf("invalid diagnostic: %+v", d)
			}
		}
		if !found {
			t.Errorf("%s: expected %q, got %+v", tt.source, tt.message, got)
		}
	}
}

func TestUnitDiagnosticsIntegration(t *testing.T) {
	d := Parse("file:///U.pas", "unit U;\ninterface\nimplementation\nend;")
	if len(d.Diagnostics) != 1 {
		t.Fatalf("diagnostics: %+v", d.Diagnostics)
	}
	if d.Diagnostics[0].Range.Start != (Position{Line: 3, Character: 3}) {
		t.Fatalf("range: %+v", d.Diagnostics[0].Range)
	}
	for _, source := range []string{"program P; begin end.", "library L; begin end.", "package P; end."} {
		if got := unitDiagnostics("file:///P.pas", source); len(got) != 0 {
			t.Fatalf("non-unit diagnostics: %+v", got)
		}
	}
}
