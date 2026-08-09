package lsp

import "testing"

func TestSummaryAndSignature(t *testing.T) {
	document := Parse("file:///test.pas", `
/// <summary>
/// Adds two values.
/// </summary>
function Add(const Left, Right: Integer): Integer;
`)
	var symbol *Symbol
	for index := range document.Symbols {
		if document.Symbols[index].Name == "Add" {
			symbol = &document.Symbols[index]
			break
		}
	}
	if symbol == nil {
		t.Fatal("function Add was not indexed")
	}
	if symbol.Detail != "function Add(const Left, Right: Integer): Integer;" {
		t.Fatalf("signature = %q", symbol.Detail)
	}
	if symbol.Documentation != "Adds two values." {
		t.Fatalf("summary = %q", symbol.Documentation)
	}
}
