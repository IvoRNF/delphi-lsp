package lsp

import "testing"

func TestUsesClauseCompletionListsOnlyUnitNames(t *testing.T) {
	main := Parse("file:///workspace/Main.dpr", `program Main;

uses
  Existing,
  Company.To

var
  CurrentVariable: Integer;
begin
end.
`)
	companyTools := Parse("file:///workspace/Company.Tools.pas", `unit Company.Tools;

interface

procedure ToolProcedure;

implementation

end.
`)
	other := Parse("file:///workspace/Other.pas", `unit Other;

interface

const OtherConstant = 1;

implementation

end.
`)

	server := NewServer(nil, nil)
	server.indexReplace(main.URI, main)
	server.indexReplace(companyTools.URI, companyTools)
	server.indexReplace(other.URI, other)
	server.noteUnit(companyTools.URI, companyTools.Text)
	server.noteUnit(other.URI, other.Text)

	items := server.completions(main.URI, Position{Line: 4, Character: len("  Company.To")})
	if len(items) != 1 || items[0].Label != "Company.Tools" {
		t.Fatalf("uses completion = %#v; want only Company.Tools", items)
	}
	if items[0].Kind != 9 || items[0].Detail != "unit Company.Tools" {
		t.Fatalf("unit completion metadata = %#v", items[0])
	}
}

func TestUsesClauseCompletionAfterCommaExcludesSymbols(t *testing.T) {
	main := Parse("file:///workspace/Main.dpr", "program Main;\n\nuses\n  Existing,\n\nvar\n  CurrentVariable: Integer;\nbegin\nend.\n")
	first := Parse("file:///workspace/First.pas", "unit First;\ninterface\nconst FirstConstant = 1;\nimplementation\nend.\n")
	second := Parse("file:///workspace/Second.pas", "unit Second;\ninterface\nprocedure SecondProcedure;\nimplementation\nend.\n")

	server := NewServer(nil, nil)
	server.indexReplace(main.URI, main)
	server.indexReplace(first.URI, first)
	server.indexReplace(second.URI, second)
	server.noteUnit(first.URI, first.Text)
	server.noteUnit(second.URI, second.Text)

	items := server.completions(main.URI, Position{Line: 3, Character: len("  Existing,")})
	if got := labels(items); len(got) != 2 || got[0] != "First" || got[1] != "Second" {
		t.Fatalf("uses completion = %#v; want [First Second]", got)
	}
}
