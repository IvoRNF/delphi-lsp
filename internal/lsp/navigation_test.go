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
