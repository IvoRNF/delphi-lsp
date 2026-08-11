package lsp

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestInitializeReply(t *testing.T) {
	json := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	input := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(json), json)
	var output bytes.Buffer
	if err := NewServer(strings.NewReader(input), &output).Serve(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"capabilities"`) {
		t.Fatalf("initialize response missing: %q", output.String())
	}
}
func TestDidOpenPublishesEmptyDiagnosticsArray(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///test.pas","languageId":"pascal","version":1,"text":"unit Test;"}}}`
	input := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	var output bytes.Buffer
	if err := NewServer(strings.NewReader(input), &output).Serve(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"diagnostics":[]`) {
		t.Fatalf("expected an empty diagnostics array, got: %q", output.String())
	}
}
