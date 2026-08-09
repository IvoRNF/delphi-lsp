package lsp

import "testing"

func TestClassAndRecordMemberDefinitions(t *testing.T) {
	document := Parse("file:///members.pas", `
type
  TCounter = class
  private
    FValue: Integer;
  public
    procedure Reset;
    property Value: Integer read FValue;
  end;
  TPair = record
    Left: Integer;
  end;

procedure TCounter.Reset;
begin
  FValue := 0;
end;
`)
	server := &Server{docs: map[string]*Document{document.URI: document}}
	field := server.definitionLocations(document, Position{Line: 14, Character: 3}, "FValue")
	if len(field) != 1 || field[0].Range.Start.Line != 4 {
		t.Fatalf("class field definition = %#v", field)
	}
	member := server.definitionLocations(document, Position{Line: 6, Character: 15}, "Reset")
	if len(member) != 1 || member[0].Range.Start.Line != 6 {
		t.Fatalf("class method definition = %#v", member)
	}
	recordField := server.definitionLocations(document, Position{Line: 10, Character: 5}, "Left")
	if len(recordField) != 1 || recordField[0].Range.Start.Line != 10 {
		t.Fatalf("record field definition = %#v", recordField)
	}
}
