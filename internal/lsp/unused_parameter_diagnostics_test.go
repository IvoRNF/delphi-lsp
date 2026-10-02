package lsp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestUnusedParameters(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         string
	}{
		{"procedure", "procedure P(Used, Unused: Integer); begin WriteLn(Used); end;", "unused"},
		{"function", "function F(A, B: Integer): Integer; begin Result := A; end;", "b"},
		{"empty body", "procedure P(A: Integer); begin end;", "a"},
		{"no parameters", "procedure P; begin end;", ""},
		{"modifiers", "procedure P(const A: string; var B: Integer; out C: Integer; constref D: string); begin B := 1; C := 2; end;", "a,d"},
		{"untyped", "procedure P(const A; var B); begin Move(A, B, 1); end;", ""},
		{"comments and strings", "procedure P(A: string); begin WriteLn('A'); { A } // A\nend;", "a"},
		{"case insensitive", "procedure P(Value: Integer); begin WriteLn(vAlUe); end;", ""},
		{"escaped identifier", "procedure P(&Type: Integer); begin WriteLn(&TYPE); end;", ""},
		{"escaped spelling", "procedure P(&Value: Integer); begin WriteLn(Value); end;", ""},
		{"qualified member", "procedure T.P(Value: Integer); begin Self.Value := 1; end;", "value"},
		{"parameter member", "procedure P(Value: TObject); begin Value.Free; end;", ""},
		{"default", "procedure P(A: string = 'A'; B: Integer = 0); begin end;", "a,b"},
		{"attributes and generics", "procedure T.F<T>([Ref] const A: TDictionary<string, T>; B: T); begin WriteLn(A); end;", "b"},
		{"procedural parameter", "procedure P(Callback: function(A: Integer; B: string): Boolean; C: Integer); begin Callback(C, ''); end;", ""},
		{"interface and implementation", "unit U; interface procedure P(A: Integer); implementation procedure P(A: Integer); begin end; end.", "a"},
		{"class declaration", "unit U; interface type T = class procedure P(A: Integer); end; implementation procedure T.P(A: Integer); begin end; end.", "a"},
		{"private types", "unit U; interface implementation type T = class procedure P(A: Integer); end; TProc = procedure(B: Integer); procedure Q(C: Integer); begin end; end.", "c"},
		{"forward", "procedure P(A: Integer); forward; procedure Q(B: Integer); begin end;", "b"},
		{"external", "procedure P(A: Integer); external 'lib'; procedure Q(B: Integer); begin end;", "b"},
		{"calling convention", "procedure P(A: Integer); stdcall; begin end;", "a"},
		{"overloads", "procedure P(A: Integer); overload; begin WriteLn(A); end; procedure P(A: string); overload; begin end;", "a"},
		{"separate routine", "procedure P(A: Integer); begin end; procedure Q; begin WriteLn(A); end;", "a"},
		{"nested capture", "procedure P(A: Integer); procedure Q; begin WriteLn(A); end; begin Q; end;", ""},
		{"nested parameter shadow", "procedure P(A: Integer); procedure Q(A: Integer); begin WriteLn(A); end; begin Q(1); end;", "a"},
		{"nested local shadow", "procedure P(A: Integer); procedure Q; var A: Integer; begin A := 1; end; begin Q; end;", "a"},
		{"nested unused", "procedure P(A: Integer); procedure Q(B: Integer); begin end; begin WriteLn(A); Q(1); end;", "b"},
		{"local procedure type", "procedure P(A: Integer); var Callback: procedure(A: Integer); begin end;", "a"},
		{"anonymous capture", "procedure P(A: Integer); begin Run(procedure begin WriteLn(A); end); end;", ""},
		{"anonymous parameter", "procedure P(A: Integer); begin Run(procedure(A: Integer) begin WriteLn(A); end); end;", "a"},
		{"anonymous unused", "procedure P; begin Run(procedure(A: Integer) begin end); end;", "a"},
		{"nested blocks", "procedure P(A: Integer); begin try case 1 of 1: begin WriteLn(A); end; end; finally Cleanup; end; end;", ""},
		{"inline shadow", "procedure P(A: Integer); begin begin var A := 1; WriteLn(A); end; end;", "a"},
		{"inline scope ends", "procedure P(A: Integer); begin begin var A := 1; end; WriteLn(A); end;", ""},
		{"initializer", "procedure P(A: Integer); begin var B := A; end;", ""},
		{"local alias", "procedure P(A: Integer); var B: Integer absolute A; begin B := 1; end;", ""},
		{"assembly", "procedure P(A: Integer); asm nop end;", ""},
		{"implicit inherited", "procedure T.P(A: Integer); begin inherited; end;", ""},
		{"explicit inherited", "procedure T.P(A: Integer); begin inherited P(1); end;", "a"},
		{"conditional", "procedure P(A: Integer); begin {$IFDEF X} WriteLn(1); {$ELSE} WriteLn(A); {$ENDIF} end;", "a"},
		{"incomplete header", "procedure P(A: Integer", ""},
		{"incomplete body", "procedure P(A: Integer); begin", ""},
		{"missing body", "procedure P(A: Integer);", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := unusedParameterTokenDiagnostics(syntaxTokens(tt.source))
			var names []string
			for _, diagnostic := range got {
				name := strings.TrimSuffix(strings.TrimPrefix(diagnostic.Message, "Unused parameter '"), "'")
				names = append(names, name)
				if diagnostic.Severity != 2 || diagnostic.Source != "delphi-lsp" || !reflect.DeepEqual(diagnostic.Tags, []int{1}) {
					t.Errorf("warning metadata: %+v", diagnostic)
				}
			}
			if strings.Join(names, ",") != tt.want {
				t.Fatalf("got %+v, want unused parameters %q", got, tt.want)
			}
		})
	}
}

func TestUnusedParameterDiagnosticIntegration(t *testing.T) {
	source := "unit U;\ninterface\nprocedure P(A: Integer);\nimplementation\nprocedure P(\n  A: Integer);\nbegin\nend;\nend."
	d := Parse("file:///U.pas", source)
	if len(d.Diagnostics) != 1 {
		t.Fatalf("diagnostics: %+v", d.Diagnostics)
	}
	diagnostic := d.Diagnostics[0]
	want := Range{Start: Position{Line: 5, Character: 2}, End: Position{Line: 5, Character: 3}}
	if diagnostic.Range != want {
		t.Fatalf("range: %+v, want %+v", diagnostic.Range, want)
	}
	encoded, err := json.Marshal(diagnostic)
	if err != nil || !strings.Contains(string(encoded), `"tags":[1]`) || !strings.Contains(string(encoded), `"severity":2`) {
		t.Fatalf("JSON: %s, err: %v", encoded, err)
	}
	// Editing the routine to use its argument clears the warning.
	d = Parse("file:///U.pas", strings.Replace(source, "begin\nend", "begin\nWriteLn(A);\nend", 1))
	if len(d.Diagnostics) != 0 {
		t.Fatalf("diagnostics after edit: %+v", d.Diagnostics)
	}
}

func TestUnusedParameterUTF16Range(t *testing.T) {
	diagnostics := unusedParameterTokenDiagnostics(syntaxTokens("procedure P({😀} Value: Integer); begin end;"))
	want := Range{Start: Position{Character: 17}, End: Position{Character: 22}}
	if len(diagnostics) != 1 || diagnostics[0].Range != want {
		t.Fatalf("UTF-16 range: %+v, want %+v", diagnostics, want)
	}
}
