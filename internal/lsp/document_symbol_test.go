package lsp

import "testing"

func TestDocumentSymbolRangesContainMultiLineParameterSelections(t *testing.T) {
	document := Parse("file:///Test.pas", `unit Test;

interface

procedure Configure(
  FirstOption: Integer;
  SecondOption: string);

implementation

end.`)

	for _, symbol := range document.Symbols {
		if !rangeContains(symbol.Range, symbol.Selection) {
			t.Fatalf("symbol %q has selection %#v outside range %#v", symbol.Name, symbol.Selection, symbol.Range)
		}
	}
}

func rangeContains(full, selection Range) bool {
	return positionBeforeOrEqual(full.Start, selection.Start) && positionBeforeOrEqual(selection.End, full.End)
}

func positionBeforeOrEqual(left, right Position) bool {
	return left.Line < right.Line || left.Line == right.Line && left.Character <= right.Character
}
