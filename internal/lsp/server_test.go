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
