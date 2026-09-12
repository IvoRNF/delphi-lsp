package lsp

import "testing"

func TestSemicolonDiagnostics(t *testing.T) {
	tests := []struct {
		name, source string
		want         int
	}{
		{"assignment", "begin\n  A := 1\n  B := 2;\nend.", 1},
		{"calls on same line", "begin FirstCall SecondCall; end.", 1},
		{"multiple missing", "begin A := 1 B := 2 C := 3 end.", 2},
		{"nested block", "begin begin A := 1 end B := 2 end.", 1},
		{"after if", "begin if Ready then Run else Stop Next end.", 1},
		{"inside loop", "begin while Ready do begin Run Next end end.", 1},
		{"after repeat", "begin repeat Run until Done Next end.", 1},
		{"case branches", "begin case N of 1: Run 2: Stop end end.", 1},
		{"initialization", "unit U; interface implementation initialization Run Next finalization Stop end.", 1},
		{"optional last separator", "begin A := 1 end.", 0},
		{"character codes", "begin S := #13#10'text'; S := 'text'#13#10'next' end.", 0},
		{"empty", "begin ; ; end.", 0},
		{"if else", "begin if Ready then Run else Stop end.", 0},
		{"dangling else", "begin if A then if B then Run else Stop; Next end.", 0},
		{"compound if", "begin if Ready then begin Run end else begin Stop end end.", 0},
		{"multiline", "begin A :=\n Foo(\n  1, 2\n )\n + 3;\n B := A\n * 2 end.", 0},
		{"comments and strings", "begin A := 'it''s ; begin end'; { end; } (* begin *) // end\n B := 'ok' end.", 0},
		{"repeat", "begin repeat Run until Done end.", 0},
		{"loops", "begin for I := 0 to 2 do Run; for X in Items do Run; with Obj do Run; while Ready do Run end.", 0},
		{"case", "begin case N of 1, 2: Run; 3..5: begin Stop end else Other end end.", 0},
		{"try finally", "begin try Run finally Stop end end.", 0},
		{"try except", "begin try Run except on E: Exception do Handle(E); on E: Other do raise else Stop end end.", 0},
		{"asm", "begin asm mov eax, 1 xor ebx, ebx end; Run end.", 0},
		{"anonymous method", "begin Run(procedure begin First; Second end); Handler := procedure begin Run end end.", 0},
		{"declarations", "unit U; interface type T = class procedure Run; end; implementation procedure T.Run; var N: Integer; begin N := 1 end; end.", 0},
		{"label", "begin Again: Run; goto Again end.", 0},
		{"inline variable", "begin var N: Integer := 1; Run(N) end.", 0},
		{"conditional branch", "begin {$IFDEF X} Run {$ELSE} Other {$ENDIF}; Next end.", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Parse("file:///Test.pas", tt.source)
			if len(d.Diagnostics) != tt.want {
				t.Fatalf("got %d diagnostics, want %d: %+v", len(d.Diagnostics), tt.want, d.Diagnostics)
			}
			for _, diagnostic := range d.Diagnostics {
				if diagnostic.Severity != 1 || diagnostic.Source != "delphi-lsp" {
					t.Fatalf("unexpected diagnostic: %+v", diagnostic)
				}
			}
		})
	}
}

func TestSemicolonDiagnosticUTF16Range(t *testing.T) {
	d := Parse("file:///Test.pas", "begin\r\n  Run('😀') // comment\r\n  Next;\r\nend.")
	if len(d.Diagnostics) != 1 {
		t.Fatalf("diagnostics: %+v", d.Diagnostics)
	}
	want := Range{Start: Position{Line: 1, Character: 10}, End: Position{Line: 1, Character: 11}}
	if d.Diagnostics[0].Range != want {
		t.Fatalf("range = %+v; want %+v", d.Diagnostics[0].Range, want)
	}
}
