package lsp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func benchmarkSource() string {
	var source strings.Builder
	source.WriteString("unit Bench;\ninterface\nimplementation\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&source, "procedure Routine%d(Value: Integer);\nvar Local: Integer;\nbegin\nLocal := Value + 1;\nend;\n", i)
	}
	source.WriteString("end.\n")
	return source.String()
}

func BenchmarkParseLargeUnit(b *testing.B) {
	source := benchmarkSource()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse("file:///Bench.pas", source)
	}
}

func BenchmarkReplaceOverloads(b *testing.B) {
	s := NewServer(strings.NewReader(""), io.Discard)
	for i := 0; i < 20; i++ {
		doc := &Document{URI: fmt.Sprintf("file:///Unit%d.pas", i)}
		for j := 0; j < 200; j++ {
			doc.Symbols = append(doc.Symbols, Symbol{Name: "Overloaded"})
		}
		s.indexDoc(doc)
	}
	doc := s.document("file:///Unit0.pas")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.indexReplace(doc.URI, doc)
	}
}

func TestConcurrentIndexFilePreservesEditorDocument(t *testing.T) {
	s := NewServer(strings.NewReader(""), io.Discard)
	path := filepath.Join(t.TempDir(), "Bench.pas")
	if err := os.WriteFile(path, []byte(benchmarkSource()), 0600); err != nil {
		t.Fatal(err)
	}
	uri := fileURI(path)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			s.indexFile(uri)
		}()
	}
	close(start)
	edited := Parse(uri, "unit Bench;\ninterface\nvar Edited: Integer;\nimplementation\nend.")
	s.indexReplace(uri, edited)
	workers.Wait()
	if s.document(uri) != edited || len(s.symbolRefs("Edited")) != 1 || len(s.symbolRefs("Routine0")) != 0 {
		t.Fatal("disk indexing overwrote or duplicated editor symbols")
	}
	if len(s.loading) != 0 {
		t.Fatal("completed loads were not released")
	}
}

func TestEnsureParsedWaitsForActiveLoad(t *testing.T) {
	s := NewServer(strings.NewReader(""), io.Discard)
	uri := "file:///Loading.pas"
	done := make(chan struct{})
	s.loading = map[string]chan struct{}{uri: done}
	returned := make(chan *Document, 1)
	go func() {
		s.ensureParsed(uri)
		returned <- s.document(uri)
	}()
	doc := Parse(uri, "unit Loading; interface implementation end.")
	s.indexDoc(doc)
	s.mu.Lock()
	delete(s.loading, uri)
	close(done)
	s.mu.Unlock()
	select {
	case got := <-returned:
		if got != doc {
			t.Fatal("request returned before the active load was published")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not resume after indexing")
	}
}

func TestOverloadReplacementKeepsOtherDocuments(t *testing.T) {
	s := NewServer(strings.NewReader(""), io.Discard)
	for _, uri := range []string{"file:///A.pas", "file:///B.pas"} {
		s.indexDoc(&Document{URI: uri, Symbols: []Symbol{{Name: "Run"}, {Name: "RUN"}}})
	}
	if len(s.docNames["file:///A.pas"]) != 1 {
		t.Fatal("overloads should contribute one cleanup key")
	}
	s.indexReplace("file:///A.pas", &Document{URI: "file:///A.pas", Symbols: []Symbol{{Name: "New"}}})
	if refs := s.symbolRefs("run"); len(refs) != 2 || refs[0].uri != "file:///B.pas" || refs[1].uri != "file:///B.pas" {
		t.Fatalf("replacement corrupted other document overloads: %v", refs)
	}
}

func TestIndexWorkersDrainQueue(t *testing.T) {
	s := NewServer(strings.NewReader(""), io.Discard)
	dir := t.TempDir()
	for i := 0; i < 32; i++ {
		path := filepath.Join(dir, fmt.Sprintf("Unit%d.pas", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("unit Unit%d; interface implementation end.", i)), 0600); err != nil {
			t.Fatal(err)
		}
		s.enqueue(fileURI(path))
		s.spawnWorkers()
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.RLock()
		done := len(s.docs) == 32 && s.active == 0 && len(s.pendingQueue) == 0 && len(s.pendingSet) == 0 && len(s.loading) == 0
		s.mu.RUnlock()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("background indexer did not drain the queue")
		}
		time.Sleep(time.Millisecond)
	}
}
