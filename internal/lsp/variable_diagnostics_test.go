package lsp

import "testing"

func TestLocalVariableSemicolons(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   int
	}{
		{"procedure P; var X: Integer begin end;", 1},
		{"function F: Integer; var X: Integer Y: String begin end;", 2},
		{"procedure P; var X,\nY:\nInteger\nbegin end;", 1},
		{"procedure P; var X: Integer // comment\nbegin end;", 1},
		{"procedure P; var X: Integer", 1},
		{"procedure P; var X: Integer procedure Q; begin end; begin end;", 1},
		{"procedure P; var X: Integer; procedure Q; var Y: Integer begin end; begin end;", 1},
		{"procedure P; var X: array[0..2] of Integer begin end;", 1},
		{"procedure P; var X: TDictionary<string, TList<Integer>> begin end;", 1},
		{"procedure P; var X: record A: Integer; B: String end begin end;", 1},
		{"procedure P; var X: Integer; Y: String; begin end;", 0},
		{"procedure P(var X: Integer; const Y: String); begin end;", 0},
		{"procedure P; var X: function(var Y: Integer): Boolean; begin end;", 0},
		{"procedure P; var X: reference to procedure; begin end;", 0},
		{"procedure P; var X: procedure of object; begin end;", 0},
		{"procedure P; var X: TDictionary<string, TList<Integer>>; begin end;", 0},
		{"procedure P; var X: record A: Integer; B: String end; begin end;", 0},
		{"procedure P; var X: Integer absolute Y; begin end;", 0},
		{"procedure P; var X: Integer; const Y = 1; begin end;", 0},
		{"procedure P; begin var X := 1; end;", 0},
		{"unit U; interface type T = class procedure P; class var X: Integer; end; implementation end.", 0},
	} {
		got := localVariableDiagnostics(tt.source)
		if len(got) != tt.want {
			t.Errorf("%s: got %+v, want %d", tt.source, got, tt.want)
		}
	}
}

func TestLocalVariableDiagnosticIntegration(t *testing.T) {
	source := "unit U;\ninterface\nimplementation\nprocedure P;\nvar X: Integer\nbegin end;\nend."
	d := Parse("file:///U.pas", source)
	if len(d.Diagnostics) != 1 {
		t.Fatalf("diagnostics: %+v", d.Diagnostics)
	}
	diagnostic := d.Diagnostics[0]
	if diagnostic.Message != "Missing ';' after local variable declaration" || diagnostic.Severity != 1 || diagnostic.Range != (Range{Start: Position{Line: 4, Character: 7}, End: Position{Line: 4, Character: 14}}) {
		t.Fatalf("diagnostic: %+v", diagnostic)
	}
}
