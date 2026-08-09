package lsp

import "testing"

func TestUsesClauseResolvesIndexedUnit(t *testing.T) {
	main := Parse("file:///main.pas", `
program Main;
uses
  Support.Unit;
`)
	unit := Parse("file:///support.pas", `
unit Support.Unit;
interface
implementation
end.
`)
	server := &Server{docs: map[string]*Document{main.URI: main, unit.URI: unit}}
	locations := server.definitionLocations(main, Position{Line: 3, Character: 5}, "Unit")
	if len(locations) != 1 || locations[0].URI != unit.URI || locations[0].Range.Start.Line != 1 {
		t.Fatalf("uses-clause definition = %#v", locations)
	}
	if reference := main.useAt(Position{Line: 3, Character: 5}); reference == nil || reference.Name != "Support.Unit" {
		t.Fatalf("uses-clause reference = %#v", reference)
	}
}

func TestRoutineHeaderResolvesToImplementation(t *testing.T) {
	document := Parse("file:///unit.pas", `
unit Unit1;
interface
function Add(Left, Right: Integer): Integer;
implementation
function Add(Left, Right: Integer): Integer;
begin
  Result := Left + Right;
end;
end.
`)
	server := &Server{docs: map[string]*Document{document.URI: document}}
	locations := server.definitionLocations(document, Position{Line: 3, Character: 10}, "Add")
	if len(locations) != 1 || locations[0].Range.Start.Line != 5 {
		t.Fatalf("routine implementation definition = %#v", locations)
	}
}

func TestConstructorAndDestructorHeadersResolveToImplementations(t *testing.T) {
	document := Parse("file:///unit.pas", `
unit Unit1;
interface
type
  TCounter = class
    constructor Create;
    destructor Destroy; override;
  end;
implementation
constructor TCounter.Create;
begin
end;

destructor TCounter.Destroy;
begin
end;
end.
`)
	server := &Server{docs: map[string]*Document{document.URI: document}}
	constructor := server.definitionLocations(document, Position{Line: 5, Character: 17}, "Create")
	if len(constructor) != 1 || constructor[0].Range.Start.Line != 9 {
		t.Fatalf("constructor implementation definition = %#v", constructor)
	}
	destructor := server.definitionLocations(document, Position{Line: 6, Character: 16}, "Destroy")
	if len(destructor) != 1 || destructor[0].Range.Start.Line != 13 {
		t.Fatalf("destructor implementation definition = %#v", destructor)
	}
}

func TestTypeAliasesAreAvailableForHover(t *testing.T) {
	document := Parse("file:///types.pas", `
unit Types;
interface
type
  TIdentifier = Integer;
  TChoice = (First, Second);
implementation
end.
`)
	server := &Server{docs: map[string]*Document{document.URI: document}}
	for _, name := range []string{"TIdentifier", "TChoice"} {
		symbol := server.symbolNamed(name)
		if symbol == nil || symbol.Kind != symbolClass {
			t.Fatalf("type hover symbol for %s = %#v", name, symbol)
		}
	}
}

func TestDefinitionLocationsDeduplicateEquivalentFileURIs(t *testing.T) {
	text := "unit Sample;\ninterface\ntype\n  TSample = class;\nimplementation\nend.\n"
	first := Parse("file:///C:/work/Sample.pas", text)
	second := Parse("file:///c:/work/sample.pas", text)
	server := &Server{docs: map[string]*Document{first.URI: first, second.URI: second}}
	locations := server.definitionLocations(first, Position{Line: 3, Character: 3}, "TSample")
	if len(locations) != 1 {
		t.Fatalf("deduplicated definitions = %#v", locations)
	}
}
