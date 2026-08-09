package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	Method  string          `json:"method,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}
type Server struct {
	in    *bufio.Reader
	out   io.Writer
	docs  map[string]*Document
	roots map[string]bool
}

func NewServer(in io.Reader, out io.Writer) *Server {
	s := &Server{in: bufio.NewReader(in), out: out, docs: map[string]*Document{}, roots: map[string]bool{}}
	if config := os.Getenv("DELPHI_LSP_CONFIG"); config != "" {
		s.addConfig(config)
	}
	return s
}

func (s *Server) Serve() error {
	for {
		m, err := s.read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.handle(m)
	}
}
func (s *Server) read() (rpcMessage, error) {
	var length int
	for {
		line, e := s.in.ReadString('\n')
		if e != nil {
			return rpcMessage{}, e
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			fmt.Sscanf(line, "Content-Length: %d", &length)
		}
	}
	if length < 1 {
		return rpcMessage{}, fmt.Errorf("missing Content-Length")
	}
	b := make([]byte, length)
	_, e := io.ReadFull(s.in, b)
	var m rpcMessage
	if e == nil {
		e = json.Unmarshal(b, &m)
	}
	return m, e
}
func (s *Server) send(v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(b), b)
}
func (s *Server) reply(id json.RawMessage, result any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}
func (s *Server) notify(method string, p any) {
	s.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": p})
}

func (s *Server) handle(m rpcMessage) {
	switch m.Method {
	case "initialize":
		var p struct {
			WorkspaceFolders []struct {
				URI string `json:"uri"`
			} `json:"workspaceFolders"`
			RootURI string `json:"rootUri"`
		}
		json.Unmarshal(m.Params, &p)
		for _, f := range p.WorkspaceFolders {
			s.roots[f.URI] = true
		}
		if p.RootURI != "" {
			s.roots[p.RootURI] = true
		}
		// Reply first: configured source trees can be large.
		s.reply(m.ID, map[string]any{"capabilities": map[string]any{"textDocumentSync": 1, "hoverProvider": true, "definitionProvider": true, "referencesProvider": true, "completionProvider": map[string]any{"triggerCharacters": []string{"."}}, "documentSymbolProvider": true, "workspaceSymbolProvider": true, "workspace": map[string]any{"workspaceFolders": map[string]bool{"supported": true, "changeNotifications": true}}}, "serverInfo": map[string]string{"name": "delphi-lsp", "version": "0.1.0"}})
	case "initialized":
		s.indexRoots()
	case "workspace/didChangeWorkspaceFolders":
		var p struct {
			Event struct {
				Added []struct {
					URI string `json:"uri"`
				} `json:"added"`
				Removed []struct {
					URI string `json:"uri"`
				} `json:"removed"`
			} `json:"event"`
		}
		json.Unmarshal(m.Params, &p)
		for _, f := range p.Event.Added {
			s.roots[f.URI] = true
		}
		for _, f := range p.Event.Removed {
			delete(s.roots, f.URI)
		}
		s.indexRoots()
	case "textDocument/didOpen", "textDocument/didChange":
		s.update(m)
	case "textDocument/didClose":
		var p struct {
			TextDocument TextDocumentIdentifier `json:"textDocument"`
		}
		json.Unmarshal(m.Params, &p)
		delete(s.docs, p.TextDocument.URI)
		s.notify("textDocument/publishDiagnostics", map[string]any{"uri": p.TextDocument.URI, "diagnostics": []Diagnostic{}})
	case "textDocument/documentSymbol":
		var p struct {
			TextDocument TextDocumentIdentifier `json:"textDocument"`
		}
		json.Unmarshal(m.Params, &p)
		d := s.docs[p.TextDocument.URI]
		out := []map[string]any{}
		if d != nil {
			for _, x := range d.Symbols {
				out = append(out, map[string]any{"name": x.Name, "detail": x.Detail, "kind": x.Kind, "range": x.Range, "selectionRange": x.Selection})
			}
		}
		s.reply(m.ID, out)
	case "workspace/symbol":
		var p struct {
			Query string `json:"query"`
		}
		json.Unmarshal(m.Params, &p)
		s.reply(m.ID, s.workspaceSymbols(p.Query))
	case "textDocument/completion":
		s.reply(m.ID, s.completions())
	case "textDocument/hover", "textDocument/definition", "textDocument/references":
		s.lookup(m)
	case "shutdown":
		s.reply(m.ID, nil)
	}
}
func (s *Server) update(m rpcMessage) {
	var p struct {
		TextDocument   TextDocumentItem `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	json.Unmarshal(m.Params, &p)
	text := p.TextDocument.Text
	if len(p.ContentChanges) > 0 {
		text = p.ContentChanges[len(p.ContentChanges)-1].Text
	}
	d := Parse(p.TextDocument.URI, text)
	s.docs[d.URI] = d
	s.notify("textDocument/publishDiagnostics", map[string]any{"uri": d.URI, "diagnostics": d.Diagnostics})
}
func (s *Server) workspaceSymbols(q string) []SymbolInformation {
	var out []SymbolInformation
	q = strings.ToLower(q)
	for uri, d := range s.docs {
		for _, x := range d.Symbols {
			if q == "" || strings.Contains(strings.ToLower(x.Name), q) {
				out = append(out, SymbolInformation{x.Name, x.Detail, x.Kind, Location{uri, x.Selection}})
			}
		}
	}
	return out
}
func (s *Server) completions() []CompletionItem {
	seen := map[string]bool{}
	out := []CompletionItem{}
	for _, d := range s.docs {
		for _, x := range d.Symbols {
			if !seen[x.Name] {
				seen[x.Name] = true
				out = append(out, CompletionItem{x.Name, x.Detail, x.Kind})
			}
		}
	}
	return out
}
func (s *Server) lookup(m rpcMessage) {
	var p TextDocumentPositionParams
	json.Unmarshal(m.Params, &p)
	d := s.docs[p.TextDocument.URI]
	if d == nil {
		s.reply(m.ID, nil)
		return
	}
	word := wordAt(d.Text, p.Position)
	hits := s.definitionLocations(d, p.Position, word)
	if m.Method == "textDocument/hover" {
		symbol := s.symbolNamed(word)
		if symbol == nil {
			s.reply(m.ID, nil)
		} else {
			contents := "```delphi\n" + symbol.Detail + "\n```"
			if symbol.Documentation != "" {
				contents += "\n\n" + symbol.Documentation
			}
			s.reply(m.ID, Hover{Contents: MarkupContent{"markdown", contents}})
		}
		return
	}
	if m.Method == "textDocument/definition" {
		s.reply(m.ID, hits)
		return
	}
	s.reply(m.ID, hits)
}
func (s *Server) indexRoots() {
	for uri := range s.roots {
		path := uriPath(uri)
		if path == "" {
			continue
		}
		filepath.WalkDir(path, func(p string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext != ".pas" && ext != ".dpr" && ext != ".dpk" {
				return nil
			}
			b, er := os.ReadFile(p)
			if er == nil {
				u := "file:///" + filepath.ToSlash(p)
				s.docs[u] = Parse(u, string(b))
			}
			return nil
		})
	}
}
func uriPath(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}
