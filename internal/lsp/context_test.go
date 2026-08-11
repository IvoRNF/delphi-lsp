package lsp

import "testing"

func TestInterfaceReceiverCompletionListsOnlyInterfaceMembers(t *testing.T) {
	document := Parse("file:///service.pas", `
unit Service;
interface
type
  IService = interface
    procedure Connect;
    function Enabled: Boolean;
  end;
  IUnrelated = interface
    procedure Disconnect;
  end;
var
  Service: IService;
implementation
procedure Use;
begin
  Service.
end;
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(document.URI, document)
	items := server.completions(document.URI, Position{Line: 16, Character: len("  Service.")})

	got := map[string]bool{}
	for _, item := range items {
		got[item.Label] = true
	}
	for _, member := range []string{"Connect", "Enabled"} {
		if !got[member] {
			t.Fatalf("missing IService member %q from %#v", member, items)
		}
	}
	for _, nonMember := range []string{"Disconnect", "Service", "IUnrelated"} {
		if got[nonMember] {
			t.Fatalf("unexpected non-member %q in %#v", nonMember, items)
		}
	}
}

func TestDefinitionPrefersCurrentUnitRoutine(t *testing.T) {
	local := Parse("file:///local.pas", `
unit Local;
interface
procedure Process;
procedure Run;
implementation
procedure Process;
begin
end;
procedure Run;
begin
  Process;
end;
end.
`)
	external := Parse("file:///external.pas", `
unit External;
interface
procedure Process;
implementation
procedure Process;
begin
end;
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(local.URI, local)
	server.indexReplace(external.URI, external)
	locations := server.definitionLocations(local, Position{Line: 11, Character: 4}, "Process")
	if len(locations) != 1 || locations[0].URI != local.URI || locations[0].Range.Start.Line != 6 {
		t.Fatalf("current unit routine definition = %#v", locations)
	}
}
func TestReceiverCompletionResolvesInterfaceFromUsedUnit(t *testing.T) {
	contracts := Parse("file:///contracts.pas", `
unit Contracts;
interface
type
  IService = interface
    procedure Connect;
  end;
  IUnrelated = interface
    procedure Disconnect;
  end;
implementation
end.
`)
	main := Parse("file:///main.pas", `
program Main;
uses
  Contracts;
var
  Service: IService;
begin
  Service.
end.
`)
	server := NewServer(nil, nil)
	server.indexReplace(contracts.URI, contracts)
	server.indexReplace(main.URI, main)
	items := server.completions(main.URI, Position{Line: 7, Character: len("  Service.")})

	got := map[string]bool{}
	for _, item := range items {
		got[item.Label] = true
	}
	if !got["Connect"] || got["Disconnect"] {
		t.Fatalf("used-unit interface completion = %#v", items)
	}
}
