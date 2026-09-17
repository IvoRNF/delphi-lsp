package lsp

import (
	"strings"
	"testing"
)

func TestConsecutiveInterfaceRoutinesRemainPublic(t *testing.T) {
	logger := Parse("file:///ClientLogger.pas", `unit ClientLogger;
interface
procedure LogText(AText: string);
procedure LogTextFmt(const AFmt: string; AArgs: array of const);
function Enabled: Boolean;
type
  TLogLevel = Integer;
implementation
procedure LogText(AText: string);
begin
end;
procedure LogTextFmt(const AFmt: string; AArgs: array of const);
begin
  LogText(AFmt);
end;
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(logger.URI, logger)
	for _, expression := range []string{"ClientLogger.", "ClientLogger.Log", "Log"} {
		main := Parse("file:///Main.pas", "unit Main;\ninterface\nuses ClientLogger;\nimplementation\nprocedure Run;\nbegin\n  "+expression+"\nend;\nend.\n")
		server.indexReplace(main.URI, main)
		items := completionKinds(server.completions(main.URI, Position{Line: 6, Character: 2 + len(expression)}))
		for _, name := range []string{"LogText", "LogTextFmt"} {
			if items[name] != 3 {
				t.Errorf("%s missing %s: %#v", expression, name, items)
			}
		}
		if expression == "ClientLogger." && (items["Enabled"] != 3 || items["TLogLevel"] != 7) {
			t.Errorf("public declarations after routines missing: %#v", items)
		}
		locations := server.definitionLocations(main, Position{Line: 6, Character: 2}, "LogTextFmt")
		if len(locations) != 1 || locations[0].URI != logger.URI || locations[0].Range.Start.Line != 3 {
			t.Errorf("LogTextFmt definition = %#v", locations)
		}
	}
	for _, symbol := range logger.Symbols {
		if symbol.Kind == symbolFunction && !symbol.Implementation && strings.HasPrefix(symbol.Name, "Log") && symbol.Owner != "" {
			t.Errorf("public routine incorrectly nested: %#v", symbol)
		}
	}
}
