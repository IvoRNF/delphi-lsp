package lsp

import "testing"

func TestDefinitionPrefersRoutineLocalSymbols(t *testing.T) {
	document := Parse("file:///scope.pas", `
procedure Run(const Input: Integer);
var
  LocalValue: Integer;
begin
  LocalValue := Input;
end;
`)
	server := &Server{docs: map[string]*Document{document.URI: document}}
	local := server.definitionLocations(document, Position{Line: 5, Character: 4}, "LocalValue")
	if len(local) != 1 || local[0].Range.Start.Line != 3 {
		t.Fatalf("local definition = %#v", local)
	}
	parameter := server.definitionLocations(document, Position{Line: 5, Character: 18}, "Input")
	if len(parameter) != 1 || parameter[0].Range.Start.Line != 1 {
		t.Fatalf("parameter definition = %#v", parameter)
	}
}

func TestDefinitionResolvesContinuedCommaSeparatedRoutineLocals(t *testing.T) {
	document := Parse("file:///comma-locals.pas", `
procedure Run;
var
  FirstValue,
  SecondValue: Integer;
  procedure Prepare;
  begin
  end;
begin
  FirstValue := SecondValue;
end;
`)
	server := NewServer(nil, nil)
	server.indexReplace(document.URI, document)

	for _, check := range []struct {
		name      string
		character int
		wantStart int
		wantLine  int
	}{
		{name: "FirstValue", character: 4, wantLine: 3, wantStart: 2},
		{name: "SecondValue", character: 18, wantLine: 4, wantStart: 2},
	} {
		locations := server.definitionLocations(document, Position{Line: 9, Character: check.character}, check.name)
		if len(locations) != 1 || locations[0].Range.Start.Line != check.wantLine || locations[0].Range.Start.Character != check.wantStart {
			t.Fatalf("definition for %s = %#v", check.name, locations)
		}
		if symbol := server.symbolAtLocation(document, locations[0]); symbol == nil || symbol.Detail != check.name+": Integer" {
			t.Fatalf("hover symbol for %s = %#v", check.name, symbol)
		}
	}
}

func TestDefinitionResolvesGlobalVariables(t *testing.T) {
	document := Parse("file:///globals.pas", `
unit Globals;
interface
var
  InterfaceValue: Integer;
procedure ReadInterfaceValue;
implementation
var
  ImplementationValue: Integer;
procedure ReadImplementationValue;
begin
  ImplementationValue := InterfaceValue;
end;
end.
`)
	server := &Server{docs: map[string]*Document{document.URI: document}}
	interfaceValue := server.definitionLocations(document, Position{Line: 11, Character: 25}, "InterfaceValue")
	if len(interfaceValue) != 1 || interfaceValue[0].Range.Start.Line != 4 {
		t.Fatalf("interface global definition = %#v", interfaceValue)
	}
	implementationValue := server.definitionLocations(document, Position{Line: 11, Character: 3}, "ImplementationValue")
	if len(implementationValue) != 1 || implementationValue[0].Range.Start.Line != 8 {
		t.Fatalf("implementation global definition = %#v", implementationValue)
	}
}
func TestDefinitionResolvesImplementationRoutineParameters(t *testing.T) {
	document := Parse("file:///parameters.pas", `
unit Parameters;
interface
procedure Declared(Value: Integer);
implementation
procedure Declared(Value: Integer);
begin
  Value := Value + 1;
end;
procedure ImplementationOnly(Value: Integer);
begin
  Value := Value + 1;
end;
end.
`)
	server := &Server{docs: map[string]*Document{document.URI: document}}

	declared := server.definitionLocations(document, Position{Line: 7, Character: 3}, "Value")
	if len(declared) != 1 || declared[0].Range.Start.Line != 5 {
		t.Fatalf("declared implementation parameter definition = %#v", declared)
	}
	implementationOnly := server.definitionLocations(document, Position{Line: 11, Character: 3}, "Value")
	if len(implementationOnly) != 1 || implementationOnly[0].Range.Start.Line != 9 {
		t.Fatalf("implementation-only parameter definition = %#v", implementationOnly)
	}
}

func TestFunctionResultIsAnImplicitLocalVariable(t *testing.T) {
	document := Parse("file:///result.pas", `
function Add(Left, Right: Integer): Integer;
begin
  Result := Left + Right;
end;
`)
	server := NewServer(nil, nil)
	server.indexReplace(document.URI, document)

	locations := server.definitionLocations(document, Position{Line: 3, Character: 4}, "Result")
	if len(locations) != 1 || locations[0].Range.Start.Line != 1 {
		t.Fatalf("implicit Result definition = %#v", locations)
	}
	items := server.completions(document.URI, Position{Line: 3, Character: 5}) // "Res|ult"
	for _, item := range items {
		if item.Label == "Result" && item.Detail == "Result: Integer" && item.Kind == 6 {
			return
		}
	}
	t.Fatalf("implicit Result missing from completion: %#v", items)
}

func TestFunctionResultCompletionPreservesAllReturnTypes(t *testing.T) {
	document := Parse("file:///result-types.pas", `
unit ResultTypes;
interface
type
  TPoint = record
    X: Integer;
  end;
  IService = interface
  end;
  TService = class
  end;
implementation
function PrimitiveValue: Integer;
begin
  Res
end;
function RecordValue: TPoint;
begin
  Res
end;
function InterfaceValue: IService;
begin
  Res
end;
function ClassValue: TService;
begin
  Res
end;
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(document.URI, document)
	for _, check := range []struct {
		line int
		want string
	}{
		{14, "Result: Integer"},
		{18, "Result: TPoint"},
		{22, "Result: IService"},
		{26, "Result: TService"},
	} {
		items := server.completions(document.URI, Position{Line: check.line, Character: len("  Res")})
		found := false
		for _, item := range items {
			if item.Label == "Result" && item.Detail == check.want && item.Kind == 6 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Result completion at line %d = %#v; want %q", check.line, items, check.want)
		}
	}
}

func TestRecordFunctionResultCompletesItsFields(t *testing.T) {
	document := Parse("file:///inventario.pas", `
unit Inventario;
interface
implementation
type
  TInventarioNFEConfig = record
    vGeracaoNFeMov: Boolean;
    vEvento, vBaseCustos, VLimiteItensNF: Integer;
  end;
function LoadConfig: TInventarioNFEConfig;
begin
  Result.
end;
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(document.URI, document)
	items := server.completions(document.URI, Position{Line: 11, Character: len("  Result.")})
	got := map[string]string{}
	for _, item := range items {
		got[item.Label] = item.Detail
	}
	for name, detail := range map[string]string{
		"vGeracaoNFeMov": "vGeracaoNFeMov: Boolean",
		"vEvento":        "vEvento: Integer",
		"vBaseCustos":    "vBaseCustos: Integer",
		"VLimiteItensNF": "VLimiteItensNF: Integer",
	} {
		if got[name] != detail {
			t.Fatalf("Result field completion %s = %#v; want %q", name, items, detail)
		}
	}
}

func TestCRLFFunctionResultCompletesRecordFields(t *testing.T) {
	document := Parse("file:///crlf-result.pas", "unit CRLF;\r\ninterface\r\nimplementation\r\ntype\r\n  TConfig = record\r\n    Enabled: Boolean;\r\n  end;\r\nfunction LoadConfig: TConfig;\r\nbegin\r\n  Result.\r\nend;\r\nend.\r\n")
	server := NewServer(nil, nil)
	server.indexReplace(document.URI, document)
	items := server.completions(document.URI, Position{Line: 9, Character: len("  Result.")})
	for _, item := range items {
		if item.Label == "Enabled" && item.Detail == "Enabled: Boolean" {
			return
		}
	}
	t.Fatalf("CRLF Result record completion = %#v", items)
}

func TestFunctionResultCompletesRecordFieldsWithoutFinalSemicolon(t *testing.T) {
	document := Parse("file:///input.pas", `
unit Input;
interface
implementation
type
  TInputGerarNFE = record
    Inventario, Pagina: Integer
  end;
function SplitGerarNFEInputId(const AId: string): TInputGerarNFE;
begin
  Result.
end;
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(document.URI, document)
	items := server.completions(document.URI, Position{Line: 10, Character: len("  Result.")})
	got := map[string]string{}
	for _, item := range items {
		got[item.Label] = item.Detail
	}
	for _, name := range []string{"Inventario", "Pagina"} {
		if got[name] != name+": Integer" {
			t.Fatalf("Result field completion %s = %#v", name, items)
		}
	}
}

func TestCommaSeparatedInlineVarDeclarationsAreIndexedInEveryScope(t *testing.T) {
	document := Parse("file:///variables.pas", `
unit Variables;
interface
var PublicFoo, PublicBar: Integer;
implementation
var PrivateFoo, PrivateBar: Integer;
procedure Run;
var LocalFoo, LocalBar: Integer;
begin
  LocalFoo := LocalBar;
end;
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(document.URI, document)

	for _, check := range []struct {
		name           string
		owner          string
		implementation bool
	}{
		{"PublicFoo", "", false}, {"PublicBar", "", false},
		{"PrivateFoo", "", true}, {"PrivateBar", "", true},
		{"LocalFoo", "Run", false}, {"LocalBar", "Run", false},
	} {
		found := false
		for _, symbol := range document.Symbols {
			if symbol.Name == check.name && symbol.Owner == check.owner && symbol.Implementation == check.implementation && symbol.Detail == check.name+": Integer" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("comma-separated declaration for %s was not indexed: %#v", check.name, document.Symbols)
		}
	}
	items := server.completions(document.URI, Position{Line: 9, Character: len("  Local")})
	got := map[string]bool{}
	for _, item := range items {
		got[item.Label] = true
	}
	if !got["LocalFoo"] || !got["LocalBar"] {
		t.Fatalf("local comma-separated variables missing from completion: %#v", items)
	}
}
