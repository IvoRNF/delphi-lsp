package lsp

import (
	"testing"
)

func TestDebugCompletionScope(t *testing.T) {
	main := Parse("file:///Z:/agent_assets/wlsp-test/main.pas", `program Main;

uses
  MyLib, Utils;

var
  Calc: TCalculator;
begin
  Calc := TCalculator.Create;
  Calc.Add(5);
  MyConst := 1;
  UtilsL;
end.
`)
	myLib := Parse("file:///Z:/agent_assets/wlsp-test/units/mylib.pas", `unit MyLib;

interface

type
  TCalculator = class
  private
    FTotal: Integer;
  public
    procedure Add(Value: Integer);
    function Total: Integer;
  end;

function AddNumbers(Left, Right: Integer): Integer;
const MyConstant = 42;

implementation

function AddNumbers(Left, Right: Integer): Integer;
begin
  Result := Left + Right;
end;

procedure TCalculator.Add(Value: Integer);
begin
  FTotal := FTotal + Value;
end;

function TCalculator.Total: Integer;
begin
  Result := FTotal;
end;

end.
`)
	utils := Parse("file:///Z:/agent_assets/wlsp-test/units/utils.pas", `unit Utils;

interface

type
  TUtility = class
  public
    procedure Run;
  end;

procedure UtilsLog(const Msg: string);

implementation

procedure UtilsLog(const Msg: string);
begin
end;

procedure TUtility.Run;
begin
end;

end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(main.URI, main)
	server.indexReplace(myLib.URI, myLib)
	server.indexReplace(utils.URI, utils)
	server.units["mylib"] = myLib.URI
	server.units["utils"] = utils.URI

	// Prefix "MyConst" in main.pas should surface MyConstant from MyLib (used unit).
	items := server.completions(main.URI, Position{Line: 10, Character: 8})
	t.Logf("prefix MyCons -> %#v", labels(items))
	// Prefix "Util" in main.pas should surface Utils + UtilsLog from used unit.
	items = server.completions(main.URI, Position{Line: 11, Character: 6})
	t.Logf("prefix Util  -> %#v", labels(items))
	// Empty prefix at "Calc.Add|" should surface locals + used-unit members + globals.
	items = server.completions(main.URI, Position{Line: 9, Character: 7})
	t.Logf("prefix empty -> %#v", labels(items))
	// Type "T" should filter globally to T* symbols.
	items = server.completions(main.URI, Position{Line: 6, Character: 7})
	t.Logf("prefix T     -> %#v", labels(items))
}

func labels(items []CompletionItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Label)
	}
	return out
}
