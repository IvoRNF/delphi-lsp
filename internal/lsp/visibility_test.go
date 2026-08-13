package lsp

import "testing"

func TestImplementationDeclarationsStayInsideTheirUnit(t *testing.T) {
	private := Parse("file:///private.pas", `
unit Private;
interface
var
  PublicValue: Integer;
procedure PublicRoutine;
implementation
var
  HiddenValue: Integer;
procedure HiddenRoutine;
begin
end;
end.
`)
	consumer := Parse("file:///consumer.pas", `
program Consumer;
uses Private;
begin
  HiddenValue := 1;
  HiddenRoutine;
  PublicValue := 1;
  PublicRoutine;
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(private.URI, private)
	server.indexReplace(consumer.URI, consumer)
	server.units["private"] = private.URI

	for _, check := range []struct {
		name string
		line int
		want bool
	}{
		{"HiddenValue", 4, false},
		{"HiddenRoutine", 5, false},
		{"PublicValue", 6, true},
		{"PublicRoutine", 7, true},
	} {
		locations := server.definitionLocations(consumer, Position{Line: check.line, Character: 3}, check.name)
		if (len(locations) > 0) != check.want {
			t.Fatalf("%s visibility locations = %#v", check.name, locations)
		}
	}

	items := server.completions(consumer.URI, Position{Line: 3, Character: 0})
	got := map[string]bool{}
	for _, item := range items {
		got[item.Label] = true
	}
	if got["HiddenValue"] || got["HiddenRoutine"] {
		t.Fatalf("implementation declarations leaked into completion: %#v", items)
	}
	if !got["PublicValue"] || !got["PublicRoutine"] {
		t.Fatalf("interface declarations missing from completion: %#v", items)
	}
}

func TestLookupAndHoverTargetMatchVariableOrRoutineUse(t *testing.T) {
	values := Parse("file:///values.pas", `
unit Values;
interface
var
  Shared: Integer;
implementation
end.
`)
	routines := Parse("file:///routines.pas", `
unit Routines;
interface
procedure Shared;
implementation
procedure Shared;
begin
end;
end.
`)
	consumer := Parse("file:///consumer.pas", `
program Consumer;
uses Values, Routines;
begin
  Shared := 1;
  Shared();
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(values.URI, values)
	server.indexReplace(routines.URI, routines)
	server.indexReplace(consumer.URI, consumer)

	variableLocations := server.definitionLocations(consumer, Position{Line: 4, Character: 4}, "Shared")
	if len(variableLocations) != 1 || variableLocations[0].URI != values.URI {
		t.Fatalf("variable definition = %#v", variableLocations)
	}
	variable := server.symbolAtLocation(consumer, variableLocations[0])
	if variable == nil || variable.Kind != symbolVariable || variable.Detail != "Shared: Integer" {
		t.Fatalf("variable hover target = %#v", variable)
	}

	routineLocations := server.definitionLocations(consumer, Position{Line: 5, Character: 4}, "Shared")
	if len(routineLocations) != 1 || routineLocations[0].URI != routines.URI {
		t.Fatalf("routine definition = %#v", routineLocations)
	}
	routine := server.symbolAtLocation(consumer, routineLocations[0])
	if routine == nil || !isRoutineSymbol(*routine) || routine.Detail != "procedure Shared;" {
		t.Fatalf("routine hover target = %#v", routine)
	}
}

func TestModuleCompletionListsOnlyThatUnitsPublicSymbols(t *testing.T) {
	module := Parse("file:///tools.pas", `
unit Company.Tools;
interface
type
  TFormatter = class
  end;
var
  PublicValue: Integer;
procedure FormatText;
implementation
var
  HiddenValue: Integer;
procedure HiddenFormat;
begin
end;
end.
`)
	consumer := Parse("file:///consumer.pas", `
program Consumer;
uses Company.Tools;
var
  UnrelatedValue: Integer;
begin
  Company.Tools.
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(module.URI, module)
	server.indexReplace(consumer.URI, consumer)
	server.units["company.tools"] = module.URI

	items := server.completions(consumer.URI, Position{Line: 6, Character: len("  Company.Tools.")})
	got := map[string]bool{}
	for _, item := range items {
		got[item.Label] = true
	}
	for _, name := range []string{"TFormatter", "PublicValue", "FormatText"} {
		if !got[name] {
			t.Fatalf("module completion is missing %s: %#v", name, items)
		}
	}
	for _, name := range []string{"HiddenValue", "HiddenFormat", "UnrelatedValue", "Company.Tools"} {
		if got[name] {
			t.Fatalf("module completion leaked %s: %#v", name, items)
		}
	}
}
