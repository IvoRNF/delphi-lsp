package lsp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestUnusedLocalVariables(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         string
	}{
		{"procedure", "procedure P; var Used, Unused: Integer; begin Used := 1; end;", "unused"},
		{"function", "function F: Integer; var Value: Integer; begin Result := 1; end;", "value"},
		{"assign and pass", "procedure P; var First, Second: Integer; begin First := 1; Consume(Second); end;", ""},
		{"case insensitive", "procedure P; var Value: Integer; begin WriteLn(vAlUe); end;", ""},
		{"escaped identifier", "procedure P; var &Type: Integer; begin WriteLn(&Type); end;", ""},
		{"member access", "procedure P; var Value: TObject; begin Value.Free; end;", ""},
		{"qualified member", "procedure T.P; var Value: Integer; begin Self.Value := 1; end;", "value"},
		{"nested capture", "procedure P; var Value: Integer; procedure Q; begin WriteLn(Value); end; begin Q; end;", ""},
		{"nested shadow", "procedure P; var Value: Integer; procedure Q; var Value: Integer; begin WriteLn(Value); end; begin Q; end;", "value"},
		{"inline", "procedure P; begin var Used := 1; WriteLn(Used); var Unused := 1; end;", "unused"},
		{"inline block", "procedure P; begin begin var Value := 1; end; end;", "value"},
		{"initializer", "procedure P(A: Integer); begin var Value := A; end;", "value"},
		{"absolute", "procedure P(A: Integer); var Alias: Integer absolute A; begin end;", "alias"},
		{"global", "var Value: Integer; procedure P; begin end;", ""},
		{"incomplete body", "procedure P; var Value: Integer; begin", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := unusedLocalVariableTokenDiagnostics(syntaxTokens(tt.source))
			var names []string
			for _, diagnostic := range got {
				name := strings.TrimSuffix(strings.TrimPrefix(diagnostic.Message, "Unused local variable '"), "'")
				names = append(names, name)
				if diagnostic.Severity != 2 || diagnostic.Source != "delphi-lsp" || !reflect.DeepEqual(diagnostic.Tags, []int{1}) {
					t.Errorf("warning metadata: %+v", diagnostic)
				}
			}
			if strings.Join(names, ",") != tt.want {
				t.Fatalf("got %+v, want unused local variables %q", got, tt.want)
			}
		})
	}
}

func TestUnusedLocalVariableDiagnosticIntegration(t *testing.T) {
	source := "unit U;\ninterface\nimplementation\nprocedure P;\nvar\n  Used, Unused: Integer;\nbegin\n  Used := 1;\nend;\nend."
	document := Parse("file:///U.pas", source)
	if len(document.Diagnostics) != 1 {
		t.Fatalf("diagnostics: %+v", document.Diagnostics)
	}
	diagnostic := document.Diagnostics[0]
	want := Range{Start: Position{Line: 5, Character: 8}, End: Position{Line: 5, Character: 14}}
	if diagnostic.Message != "Unused local variable 'unused'" || diagnostic.Range != want {
		t.Fatalf("diagnostic: %+v, want range %+v", diagnostic, want)
	}
	encoded, err := json.Marshal(diagnostic)
	if err != nil || !strings.Contains(string(encoded), `"tags":[1]`) || !strings.Contains(string(encoded), `"severity":2`) {
		t.Fatalf("JSON: %s, err: %v", encoded, err)
	}
	// Editing the routine to use the local variable clears the warning.
	document = Parse("file:///U.pas", strings.Replace(source, "Used := 1;", "Used := Unused;", 1))
	if len(document.Diagnostics) != 0 {
		t.Fatalf("diagnostics after edit: %+v", document.Diagnostics)
	}
}
