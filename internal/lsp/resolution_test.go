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
