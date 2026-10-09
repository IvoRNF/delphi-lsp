package lsp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestFunctionResultAssignmentDiagnostics(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         string
	}{
		{"assigns Result", "function F: Integer; begin Result := 1; end;", ""},
		{"assigns lowercase", "function F: Integer; begin result := 1; end;", ""},
		{"assigns function name", "function F: Integer; begin F := 1; end;", ""},
		{"assigns via Exit", "function F: Integer; begin Exit(1); end;", ""},
		{"Exit without value", "function F: Integer; begin Exit; end;", "f"},
		{"empty body", "function F: Integer; begin end;", "f"},
		{"only reads Result in expression", "function F: Integer; begin if Result > 0 then Run; end;", "f"},
		{"argument may be a var parameter", "function F: Integer; begin WriteLn(Result); end;", ""},
		{"member write", "function F: TPoint; begin Result.X := 1; end;", ""},
		{"element write", "function F: TArray<Integer>; begin Result[0] := 1; end;", ""},
		{"var parameter idiom", "function F: string; begin SetLength(Result, 4); end;", ""},
		{"raise only", "function F: Integer; begin raise E.Create('x'); end;", ""},
		{"raise after work", "function F: Integer; begin Log; raise E.Create('x'); end;", ""},
		{"conditional raise", "function F: Integer; begin if B then raise E.Create('x'); end;", "f"},
		{"retry then raise", "function F: Integer; begin if B then Run; raise E.Create('x'); end;", ""},
		{"assembler body", "function F: Integer; asm mov eax, 1 end;", ""},
		{"nested asm does not hide", "function F: Integer; begin asm nop end; end;", "f"},
		{"procedure", "procedure P; begin end;", ""},
		{"constructor", "constructor T.Create; begin end;", ""},
		{"interface declaration", "unit U; interface function F: Integer; implementation end.", ""},
		{"forward", "function F: Integer; forward; function G: Integer; begin Result := 1; end;", ""},
		{"external", "function F: Integer; external 'lib'; function G: Integer; begin Result := 1; end;", ""},
		{"two functions", "function F: Integer; begin end; function G: Integer; begin Result := 1; end;", "f"},
		{"nested function owns Result", "function Outer: Integer; function Inner: Integer; begin Result := 1; end; begin Inner; end;", "outer"},
		{"nested function missing Result", "function Outer: Integer; function Inner: Integer; begin end; begin Result := 1; end;", "inner"},
		{"nested procedure shares Result", "function Outer: Integer; procedure Inner; begin Result := 1; end; begin Inner; end;", ""},
		{"anonymous procedure shares Result", "function Outer: Integer; begin Run(procedure begin Result := 1; end); end;", ""},
		{"anonymous function owns Result", "function Outer: Integer; begin Run(function: Integer begin Result := 1; end); end;", "outer"},
		{"custom out parameter", "function Outer: Integer; begin Load(Outer); end;", ""},
		{"method implementation", "unit U; interface type T = class function F: Integer; end; implementation function T.F: Integer; begin end; end.", "f"},
		{"overload without Result", "function F(A: Integer): Integer; overload; begin end; function F(A: string): Integer; overload; begin Result := 1; end;", "f"},
		{"incomplete body", "function F: Integer; begin", ""},
		{"incomplete header", "function F", ""},
		{"empty source", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := resultTokenDiagnostics(syntaxTokens(tt.source))
			var names []string
			for _, diagnostic := range got {
				name := strings.TrimSuffix(strings.TrimPrefix(diagnostic.Message, resultWarningPrefix), "' does not assign a value to 'Result'")
				names = append(names, name)
				if diagnostic.Severity != 2 || diagnostic.Source != "delphi-lsp" || diagnostic.Tags != nil {
					t.Errorf("warning metadata: %+v", diagnostic)
				}
			}
			if strings.Join(names, ",") != tt.want {
				t.Fatalf("got %+v, want function results %q", got, tt.want)
			}
		})
	}
}

func TestFunctionResultDoesNotPolluteUnusedWarnings(t *testing.T) {
	tokens := syntaxTokens("function F(A: Integer; B: Integer): Integer; begin end;")
	if got := unusedParameterTokenDiagnostics(tokens); len(got) != 2 {
		t.Fatalf("unused parameters: %+v", got)
	}
	if got := resultTokenDiagnostics(tokens); len(got) != 1 {
		t.Fatalf("result warnings: %+v", got)
	}
}

func TestFunctionResultRealisticUnit(t *testing.T) {
	document := Parse("file:///Inventory.pas", `unit Inventory;
interface
type
  TInventory = class
  private
    FCount: Integer;
  public
    function Total: Integer;
    function Find(const Name: string): Integer;
    procedure Reset;
  end;
function Compute(const Values: array of Integer): Integer;
implementation
function TInventory.Total: Integer;
begin
  Result := FCount;
end;
function TInventory.Find(const Name: string): Integer;
var
  I: Integer;
begin
  for I := 0 to FCount - 1 do
    if I = 0 then
      Exit(I);
  raise ENotFound.Create(Name);
end;
procedure TInventory.Reset;
begin
  FCount := 0;
end;
function Compute(const Values: array of Integer): Integer;
var
  Value: Integer;
begin
  for Value in Values do
    Result := Value;
end;
function Missing: Integer;
begin
  WriteLn('x');
end;
end.
`)
	if len(document.Diagnostics) != 1 {
		t.Fatalf("diagnostics: %+v", document.Diagnostics)
	}
	diagnostic := document.Diagnostics[0]
	if diagnostic.Message != "Function 'missing' does not assign a value to 'Result'" || diagnostic.Range.Start.Line != 37 {
		t.Fatalf("diagnostic: %+v", diagnostic)
	}
}

func TestFunctionResultDiagnosticIntegration(t *testing.T) {
	source := "unit U;\ninterface\nimplementation\nfunction F(Value: Integer): Integer;\nbegin\n  Value := 1;\nend;\nend."
	document := Parse("file:///U.pas", source)
	if len(document.Diagnostics) != 1 {
		t.Fatalf("diagnostics: %+v", document.Diagnostics)
	}
	diagnostic := document.Diagnostics[0]
	want := Range{Start: Position{Line: 3, Character: 9}, End: Position{Line: 3, Character: 10}}
	if diagnostic.Message != "Function 'f' does not assign a value to 'Result'" || diagnostic.Range != want {
		t.Fatalf("diagnostic: %+v, want range %+v", diagnostic, want)
	}
	encoded, err := json.Marshal(diagnostic)
	if err != nil || !strings.Contains(string(encoded), `"severity":2`) {
		t.Fatalf("JSON: %s, err: %v", encoded, err)
	}
	// Assigning the implicit result clears the warning.
	document = Parse("file:///U.pas", strings.Replace(source, "Value := 1;", "Result := Value;", 1))
	if len(document.Diagnostics) != 0 {
		t.Fatalf("diagnostics after edit: %+v", document.Diagnostics)
	}
}

func TestFunctionResultUTF16Range(t *testing.T) {
	diagnostics := resultTokenDiagnostics(syntaxTokens("function {😀} F: Integer; begin end;"))
	want := Range{Start: Position{Line: 0, Character: 14}, End: Position{Line: 0, Character: 15}}
	if len(diagnostics) != 1 || diagnostics[0].Range != want {
		t.Fatalf("UTF-16 range: %+v, want %+v", diagnostics, want)
	}
}

func TestBodyAlwaysRaises(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         bool
	}{
		{"raise", "raise E.Create('x');", true},
		{"raise no semicolon", "raise E.Create('x')", true},
		{"work then raise", "Log; raise E.Create('x');", true},
		{"conditional raise", "if B then raise E.Create('x');", false},
		{"raise then work", "raise E.Create('x'); Log;", false},
		{"empty", "", false},
		{"ends with if", "if B then begin Run; raise E.Create('x'); end;", false},
		{"ends with begin raise", "Run; begin raise E.Create('x'); end;", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := bodyAlwaysRaises(syntaxTokens(tt.source)); got != tt.want {
				t.Fatalf("bodyAlwaysRaises(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

func TestFunctionResultSeverityMetadataIsStable(t *testing.T) {
	diagnostics := resultTokenDiagnostics(syntaxTokens("function F: Integer; begin end;"))
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics: %+v", diagnostics)
	}
	if !reflect.DeepEqual(diagnostics[0].Tags, []int(nil)) {
		t.Fatalf("result warnings must not be tagged unnecessary: %+v", diagnostics[0])
	}
}
