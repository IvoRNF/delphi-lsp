package lsp

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"testing"
)

func TestExternalUnitSnapshot(t *testing.T) {
	path := os.Getenv("DELPHI_LSP_BENCH_FILE")
	if path == "" {
		t.Skip("set DELPHI_LSP_BENCH_FILE to a Pascal source file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := Parse(fileURI(path), string(data))
	snapshot, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("symbols=%d diagnostics=%d snapshot=%x", len(doc.Symbols), len(doc.Diagnostics), sha256.Sum256(snapshot))
}

// Opt in to benchmarking a private source file without adding it to the repo.
func BenchmarkParseExternalUnit(b *testing.B) {
	path := os.Getenv("DELPHI_LSP_BENCH_FILE")
	if path == "" {
		b.Skip("set DELPHI_LSP_BENCH_FILE to a Pascal source file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	source := string(data)
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse(fileURI(path), source)
	}
}
