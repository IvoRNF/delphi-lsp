package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsedUnitLoadsOnDemandForDefinitionAndInterfaceCompletion(t *testing.T) {
	unitDir := t.TempDir()
	unitPath := filepath.Join(unitDir, "ServerCfgs.pas")
	unitText := `unit ServerCfgs;
interface
type
  IConfigSrv=interface
  ['{69E4572F-E8C4-4B41-92F3-086EF92B76DB}']
    function OpenParams(const Params: array of String): Boolean;
    function ReadParamBol(const ParamName: String; DefVal: Boolean): Boolean;
  end;
function GetConfigSrv: IConfigSrv;
implementation
end.
`
	if err := os.WriteFile(unitPath, []byte(unitText), 0o644); err != nil {
		t.Fatal(err)
	}
	main := Parse("file:///main.pas", `unit Main;
interface
uses ServerCfgs;
implementation
procedure Run;
var
  ConfigSrv: IConfigSrv;
begin
  ConfigSrv := GetConfigSrv;
  ConfigSrv.
end;
end.
`)
	server := NewServer(nil, nil)
	server.roots[fileURI(unitDir)] = true
	server.indexReplace(main.URI, main)

	definition := server.definitionLocations(main, Position{Line: 8, Character: len("  ConfigSrv := GetConfigSrv")}, "GetConfigSrv")
	if len(definition) != 1 || definition[0].URI != fileURI(unitPath) || definition[0].Range.Start.Line != 8 {
		t.Fatalf("GetConfigSrv definition = %#v", definition)
	}

	items := completionKinds(server.completions(main.URI, Position{Line: 9, Character: len("  ConfigSrv.")}))
	if items["OpenParams"] != 2 || items["ReadParamBol"] != 2 {
		t.Fatalf("IConfigSrv member completion = %#v", items)
	}
}

func TestServerCfgsPublicSymbolsWithInlineConditionalUses(t *testing.T) {
	contracts := Parse("file:///ServerCfgs.pas", `unit ServerCfgs;
interface
uses SysUtils, Classes
  {$IFDEF VER180},Variants{$ENDIF};
type
  IConfigSrv=interface
    function OpenParams(const Params: array of String): Boolean;
  end;
  RCurrentParam = record
    Name: string;
  end;
  TConfigSrv = class(TInterfacedObject, IConfigSrv)
    procedure Close;
  end;
function GetConfigSrv: IConfigSrv;
implementation
const PadErroMsg: String = 'Private';
type
  TPrivateAlias = Integer;
  TPrivateRecord = record
    Value: Integer;
  end;
function GetConfigSrv: IConfigSrv;
begin
  Result := nil;
end;
end.
`)
	main := Parse("file:///main.pas", `unit Main;
interface
uses ServerCfgs;
implementation
procedure Run;
var
  ConfigSrv: IConfigSrv;
begin
  ConfigSrv := ServerCfgs.GetConfigSrv;
  ConfigSrv.OpenParams([]);
end;
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(main.URI, main)
	server.indexReplace(contracts.URI, contracts)
	for _, prefix := range []string{"", "Get", "I", "R"} {
		edited := Parse(main.URI, strings.Replace(main.Text, "ServerCfgs.GetConfigSrv", "ServerCfgs."+prefix, 1))
		server.indexReplace(main.URI, edited)
		items := completionKinds(server.completions(main.URI, Position{Line: 8, Character: len("  ConfigSrv := ServerCfgs.") + len(prefix)}))
		for name, kind := range map[string]int{"GetConfigSrv": 3, "IConfigSrv": 7, "RCurrentParam": 7, "TConfigSrv": 7} {
			if strings.HasPrefix(name, prefix) && items[name] != kind {
				t.Errorf("ServerCfgs.%s missing %s: %#v", prefix, name, items)
			}
		}
		for _, name := range []string{"OpenParams", "Close", "Name", "Result", "PadErroMsg", "TPrivateAlias", "TPrivateRecord"} {
			if _, ok := items[name]; ok {
				t.Errorf("unit completion leaked %s: %#v", name, items)
			}
		}
	}
	server.indexReplace(main.URI, main)
	items := completionKinds(server.completions(main.URI, Position{Line: 9, Character: len("  ConfigSrv.")}))
	if items["OpenParams"] != 2 {
		t.Errorf("interface members = %#v", items)
	}
	locations := server.definitionLocations(main, Position{Line: 8, Character: len("  ConfigSrv := ServerCfgs.Get")}, "GetConfigSrv")
	if len(locations) != 1 || locations[0].Range.Start.Line != 14 {
		t.Errorf("GetConfigSrv definition = %#v", locations)
	}
}
