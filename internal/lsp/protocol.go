package lsp

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Text       string `json:"text"`
	Version    int    `json:"version"`
}
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type SymbolInformation struct {
	Name     string   `json:"name"`
	Detail   string   `json:"detail,omitempty"`
	Kind     int      `json:"kind"`
	Location Location `json:"location"`
}
type CompletionItem struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	Kind   int    `json:"kind,omitempty"`
}
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}
