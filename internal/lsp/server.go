package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// maxIndexWorkers bounds the number of files parsed concurrently by the
// background indexer. Parsing is CPU-bound regex work, so a small pool is
// enough and keeps memory usage predictable.
const maxIndexWorkers = 4

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	Method  string          `json:"method,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// symbolRef is an indexed occurrence of a symbol in the workspace.
type symbolRef struct {
	uri    string
	symbol Symbol
}

type Server struct {
	in  *bufio.Reader
	out io.Writer

	mu    sync.RWMutex
	docs  map[string]*Document
	roots map[string]bool

	// byName indexes every indexed symbol by its lowercased name so global
	// lookups (definition, hover, workspace symbols) are O(1) instead of
	// scanning every document on every request.
	byName map[string][]symbolRef
	// docNames tracks, per document URI, the lowercased names it contributed
	// to byName so edits can remove stale entries without a full scan.
	docNames map[string][]string
	// units maps a lowercased unit name to the URI that declares it. It is
	// filled by the background indexer and lets uses-clause navigation
	// resolve without a full parse of the target file.
	units     map[string]string
	unitNames map[string]string
	// pendingQueue/pendingSet hold files awaiting background indexing.
	// Lazy loading: files are queued during the workspace walk and parsed in
	// the background; a request that needs an unparsed file parses it on
	// demand instead of waiting for the whole workspace.
	pendingQueue []string
	pendingSet   map[string]bool
	active       int
}

func NewServer(in io.Reader, out io.Writer) *Server {
	s := &Server{
		in:         bufio.NewReader(in),
		out:        out,
		docs:       map[string]*Document{},
		roots:      map[string]bool{},
		byName:     map[string][]symbolRef{},
		docNames:   map[string][]string{},
		units:      map[string]string{},
		unitNames:  map[string]string{},
		pendingSet: map[string]bool{},
	}
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
		// Reply first: configured source trees can be large. Indexing is
		// deferred to the background so the client never waits on startup.
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
		s.closeDocument(p.TextDocument.URI)
		s.notify("textDocument/publishDiagnostics", map[string]any{"uri": p.TextDocument.URI, "diagnostics": []Diagnostic{}})
	case "textDocument/documentSymbol":
		var p struct {
			TextDocument TextDocumentIdentifier `json:"textDocument"`
		}
		json.Unmarshal(m.Params, &p)
		out := []map[string]any{}
		if d := s.document(p.TextDocument.URI); d != nil {
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
		var p TextDocumentPositionParams
		json.Unmarshal(m.Params, &p)
		s.reply(m.ID, s.completions(p.TextDocument.URI, p.Position))
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
	s.indexReplace(d.URI, d)
	s.notify("textDocument/publishDiagnostics", map[string]any{"uri": d.URI, "diagnostics": d.Diagnostics})
}

// closeDocument drops a closed document from the open set and the symbol
// index, then re-queues the file for background indexing so workspace
// features keep seeing it after the client closes the tab.
func (s *Server) closeDocument(uri string) {
	s.mu.Lock()
	for _, key := range s.docNames[uri] {
		refs := s.byName[key]
		out := refs[:0]
		for _, r := range refs {
			if r.uri != uri {
				out = append(out, r)
			}
		}
		if len(out) == 0 {
			delete(s.byName, key)
		} else {
			s.byName[key] = out
		}
	}
	delete(s.docNames, uri)
	delete(s.docs, uri)
	s.mu.Unlock()
	s.enqueue(uri)
	s.spawnWorkers()
}

// workspaceSymbols answers a workspace/symbol query by walking the indexed
// names (unique per document) instead of rescanning every document.
func (s *Server) workspaceSymbols(q string) []SymbolInformation {
	q = strings.ToLower(q)
	s.mu.RLock()
	names := make([]string, 0, len(s.byName))
	for key := range s.byName {
		if q != "" && !strings.Contains(key, q) {
			continue
		}
		names = append(names, key)
	}
	s.mu.RUnlock()
	sort.Strings(names)
	var out []SymbolInformation
	for _, key := range names {
		for _, ref := range s.symbolRefs(key) {
			if ref.symbol.Implementation {
				continue
			}
			out = append(out, SymbolInformation{ref.symbol.Name, ref.symbol.Detail, ref.symbol.Kind, Location{ref.uri, ref.symbol.Selection}})
		}
	}
	return out
}

// completions limits the completion scope: symbols from the current document
// first, then its used units, then the rest of the workspace. When the cursor
// sits after a partial identifier the result is filtered by that prefix, and
// the total is capped so huge workspaces do not flood the client.
func (s *Server) completions(uri string, position Position) []CompletionItem {
	const limit = 1000
	prefix := ""
	current := s.document(uri)
	if current != nil {
		if usesClauseAt(current.Lines, position) {
			return s.unitCompletions(unitPrefixAt(current.Lines, position))
		}
		prefix = strings.ToLower(prefixAt(current.Lines, position))
		if typeName := s.memberCompletionType(current, position); typeName != "" {
			return s.memberCompletions(typeName, prefix)
		}
		if moduleURI := s.moduleCompletionURI(current, position); moduleURI != "" {
			return s.moduleCompletions(moduleURI, prefix)
		}
	}
	seen := map[string]bool{}
	out := make([]CompletionItem, 0, 256)
	add := func(name, detail string, kind int) {
		if len(out) >= limit {
			return
		}
		key := strings.ToLower(name)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, CompletionItem{Label: name, Detail: detail, Kind: kind})
	}
	if d := current; d != nil {
		routine := routineAt(d, position)
		for i := range d.Symbols {
			sym := &d.Symbols[i]
			if sym.Owner != "" && (routine == nil || !strings.EqualFold(sym.Owner, routine.Name)) {
				continue
			}
			add(sym.Name, sym.Detail, completionKind(*sym))
		}
		for _, u := range d.Uses {
			unitURI := s.unitURI(u.Name)
			if unitURI == "" {
				continue
			}
			s.ensureParsed(unitURI)
			if du := s.document(unitURI); du != nil {
				for i := range du.Symbols {
					sym := &du.Symbols[i]
					if sym.Implementation || sym.Owner != "" {
						continue
					}
					add(sym.Name, sym.Detail, completionKind(*sym))
				}
			}
		}
	}
	s.mu.RLock()
	names := make([]string, 0, len(s.byName))
	for key := range s.byName {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		names = append(names, key)
	}
	s.mu.RUnlock()
	sort.Strings(names)
	for _, key := range names {
		if seen[key] {
			continue
		}
		refs := s.visibleSymbolRefs(current, key)
		if len(refs) == 0 {
			continue
		}
		seen[key] = true
		out = append(out, CompletionItem{Label: refs[0].symbol.Name, Detail: refs[0].symbol.Detail, Kind: completionKind(refs[0].symbol)})
		if len(out) >= limit {
			break
		}
	}
	return out
}

// unitCompletions lists only units known to the workspace. Unit declarations
// are published before their files are fully parsed, so this remains useful
// while the background indexer is still running.
func (s *Server) unitCompletions(prefix string) []CompletionItem {
	const limit = 1000
	prefix = strings.ToLower(prefix)

	s.mu.RLock()
	names := make(map[string]string, len(s.units)+len(s.docs))
	for key := range s.units {
		name := s.unitNames[key]
		if name == "" {
			name = key
		}
		names[key] = name
	}
	for _, document := range s.docs {
		for _, symbol := range document.Symbols {
			if symbol.Kind == symbolModule {
				names[strings.ToLower(symbol.Name)] = symbol.Name
			}
		}
	}
	s.mu.RUnlock()

	keys := make([]string, 0, len(names))
	for key := range names {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	out := make([]CompletionItem, 0, len(keys))
	for _, key := range keys {
		name := names[key]
		out = append(out, CompletionItem{Label: name, Detail: "unit " + name, Kind: 9}) // Module
	}
	return out
}

// moduleCompletionURI returns the unit selected by a qualified expression
// such as `Math.`. Type-member completion takes precedence when a local
// variable has the same spelling as its unit.
func (s *Server) moduleCompletionURI(document *Document, position Position) string {
	name := qualifiedReceiverAt(document.Lines, position)
	if name == "" {
		return ""
	}
	if location := s.unitLocation(name); location != nil {
		return location.URI
	}
	return ""
}

// qualifiedReceiverAt is like receiverAt but preserves dots in a unit name,
// allowing scoped units such as Company.Tools.Text.`
func qualifiedReceiverAt(lines []string, position Position) string {
	if position.Line < 0 || position.Line >= len(lines) {
		return ""
	}
	line := lines[position.Line]
	character := position.Character
	if character > len(line) {
		character = len(line)
	}
	for character > 0 && isWordChar(line[character-1]) {
		character--
	}
	if character == 0 || line[character-1] != '.' {
		return ""
	}
	end := character - 1
	start := end
	for start > 0 && (isWordChar(line[start-1]) || line[start-1] == '.') {
		start--
	}
	return strings.Trim(line[start:end], ".")
}

// moduleCompletions lists a unit's public, top-level declarations only. Class
// members require a class receiver; locals and implementation declarations
// are not visible through a unit namespace.
func (s *Server) moduleCompletions(uri, prefix string) []CompletionItem {
	document := s.document(uri)
	if document == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []CompletionItem
	for _, symbol := range document.Symbols {
		if symbol.Owner != "" || symbol.Implementation || symbol.Kind == symbolModule {
			continue
		}
		key := strings.ToLower(symbol.Name)
		if (prefix != "" && !strings.HasPrefix(key, prefix)) || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, CompletionItem{Label: symbol.Name, Detail: symbol.Detail, Kind: completionKind(symbol)})
	}
	return out
}

// memberCompletionType resolves the identifier immediately before a dot to
// its declared type. A non-empty result makes completion member-only instead
// of mixing in unrelated workspace symbols.
func (s *Server) memberCompletionType(document *Document, position Position) string {
	return s.memberTypeAt(document, position)
}

func (s *Server) memberCompletions(typeName, prefix string) []CompletionItem {
	seenTypes := map[string]bool{}
	seenMembers := map[string]bool{}
	var out []CompletionItem
	var addType func(string)
	addType = func(name string) {
		key := strings.ToLower(name)
		if key == "" || seenTypes[key] {
			return
		}
		seenTypes[key] = true
		for _, typeRef := range s.symbolRefs(name) {
			document := s.document(typeRef.uri)
			if document == nil {
				continue
			}
			for _, symbol := range document.Symbols {
				if symbol.Kind == symbolClass && strings.EqualFold(symbol.Name, name) {
					for _, parent := range symbol.Parents {
						addType(parent)
					}
				}
				if !strings.EqualFold(symbol.Owner, name) || (prefix != "" && !strings.HasPrefix(strings.ToLower(symbol.Name), prefix)) {
					continue
				}
				memberKey := strings.ToLower(symbol.Name)
				if !seenMembers[memberKey] {
					seenMembers[memberKey] = true
					out = append(out, CompletionItem{Label: symbol.Name, Detail: symbol.Detail, Kind: completionKind(symbol)})
				}
			}
		}
	}
	addType(typeName)
	return out
}

// completionKind uses LSP CompletionItemKind values, which differ from the
// SymbolKind values used by document/workspace symbol requests.
func completionKind(symbol Symbol) int {
	switch symbol.Kind {
	case symbolMethod:
		return 2 // Method
	case symbolFunction:
		return 3 // Function
	case symbolField:
		return 5 // Field (a member variable)
	case symbolVariable:
		return 6 // Variable
	case symbolClass:
		return 7 // Class / interface
	case symbolModule:
		return 9 // Module
	case symbolProperty:
		return 10 // Property
	case symbolConstant:
		return 21 // Constant
	default:
		return 1 // Text
	}
}
func (s *Server) lookup(m rpcMessage) {
	var p TextDocumentPositionParams
	json.Unmarshal(m.Params, &p)
	d := s.document(p.TextDocument.URI)
	if d == nil {
		s.reply(m.ID, nil)
		return
	}
	word := wordAt(d.Lines, p.Position)
	hits := s.definitionLocations(d, p.Position, word)
	if m.Method == "textDocument/hover" {
		if unit := d.useAt(p.Position); unit != nil {
			contents := "```delphi\nunit " + unit.Name + "\n```"
			if location := s.unitLocation(unit.Name); location != nil {
				contents += "\n\nSource: " + location.URI
			}
			hoverRange := unit.Range
			s.reply(m.ID, Hover{Contents: MarkupContent{"markdown", contents}, Range: &hoverRange})
			return
		}
		var symbol *Symbol
		if len(hits) > 0 {
			symbol = s.symbolAtLocation(d, hits[0])
		}
		if symbol == nil {
			symbol = s.symbolNamed(word)
		}
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

// indexRoots walks every workspace root, queues the source files for the
// background indexer and returns immediately. The walk itself reads no file
// contents, so startup stays fast even for large trees.
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
			s.enqueue("file:///" + filepath.ToSlash(p))
			return nil
		})
	}
	s.spawnWorkers()
}

func (s *Server) enqueue(uri string) {
	s.mu.Lock()
	if !s.pendingSet[uri] {
		s.pendingSet[uri] = true
		s.pendingQueue = append(s.pendingQueue, uri)
	}
	s.mu.Unlock()
}

func (s *Server) spawnWorkers() {
	s.mu.Lock()
	if s.active >= maxIndexWorkers || len(s.pendingQueue) == 0 {
		s.mu.Unlock()
		return
	}
	want := maxIndexWorkers - s.active
	s.active += want
	s.mu.Unlock()
	for i := 0; i < want; i++ {
		go s.indexWorker()
	}
}

func (s *Server) indexWorker() {
	defer func() {
		s.mu.Lock()
		s.active--
		s.mu.Unlock()
	}()
	for {
		s.mu.Lock()
		if len(s.pendingQueue) == 0 {
			s.mu.Unlock()
			return
		}
		// Pop under the lock so two workers can never parse the same file.
		uri := s.pendingQueue[0]
		s.pendingQueue = s.pendingQueue[1:]
		delete(s.pendingSet, uri)
		s.mu.Unlock()
		s.indexFile(uri)
	}
}

// indexFile parses one file (once, from disk) and publishes its unit name and
// symbols to the index.
func (s *Server) indexFile(uri string) {
	path := uriPath(uri)
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	text := string(b)
	s.noteUnit(uri, text)
	s.indexDoc(Parse(uri, text))
}

// noteUnit publishes the unit declared at the top of a file without a full
// parse, so uses-clause navigation resolves early.
func (s *Server) noteUnit(uri, text string) {
	name := unitNameOf(text)
	if name == "" {
		return
	}
	s.mu.Lock()
	key := strings.ToLower(name)
	s.units[key] = uri
	s.unitNames[key] = name
	s.mu.Unlock()
}

// indexDoc inserts a parsed document into the index unless a newer copy (an
// open/edited document) already exists.
func (s *Server) indexDoc(doc *Document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.docs[doc.URI]; ok {
		return
	}
	s.insertIndexed(doc)
}

// insertIndexed publishes a document and its symbols. Callers must hold mu.
func (s *Server) insertIndexed(doc *Document) {
	s.docs[doc.URI] = doc
	names := s.docNames[doc.URI]
	for i := range doc.Symbols {
		sym := doc.Symbols[i]
		key := strings.ToLower(sym.Name)
		s.byName[key] = append(s.byName[key], symbolRef{uri: doc.URI, symbol: sym})
		names = append(names, key)
	}
	s.docNames[doc.URI] = names
}

// indexReplace swaps the indexed entries for a document after an edit.
func (s *Server) indexReplace(uri string, doc *Document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range s.docNames[uri] {
		refs := s.byName[key]
		out := refs[:0]
		for _, r := range refs {
			if r.uri != uri {
				out = append(out, r)
			}
		}
		if len(out) == 0 {
			delete(s.byName, key)
		} else {
			s.byName[key] = out
		}
	}
	delete(s.docNames, uri)
	delete(s.pendingSet, uri)
	s.docs[uri] = doc
	s.insertIndexed(doc)
}

// ensureParsed lazily parses a queued file on demand so a request that needs
// it never waits for the background pass to reach it.
func (s *Server) ensureParsed(uri string) {
	s.mu.RLock()
	_, done := s.docs[uri]
	queued := s.pendingSet[uri]
	s.mu.RUnlock()
	if done || !queued {
		return
	}
	s.indexFile(uri)
}

func (s *Server) document(uri string) *Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.docs[uri]
}

func (s *Server) unitURI(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.units[strings.ToLower(name)]
}

// symbolRefs returns all indexed occurrences of a symbol name. The slice is
// copied so a concurrent background insert can never invalidate it.
func (s *Server) symbolRefs(name string) []symbolRef {
	key := strings.ToLower(name)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.byName) > 0 {
		refs := s.byName[key]
		out := make([]symbolRef, len(refs))
		copy(out, refs)
		return out
	}
	// Fallback for servers constructed directly (tests) or before the first
	// background insert: scan the open documents.
	var out []symbolRef
	for _, d := range s.docs {
		for _, sym := range d.Symbols {
			if strings.EqualFold(sym.Name, name) {
				out = append(out, symbolRef{uri: d.URI, symbol: sym})
			}
		}
	}
	return out
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
