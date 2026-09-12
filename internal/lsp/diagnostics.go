package lsp

import (
	"strings"
	"unicode"
)

type syntaxToken struct {
	text string
	span Range
}

// Tokenize independently of physical lines: Pascal statements and expressions
// can span lines, and punctuation inside comments/strings is not syntax.
func syntaxTokens(source string) []syntaxToken {
	runes := []rune(source)
	var tokens []syntaxToken
	pos := Position{}
	advance := func(r rune) {
		if r == '\n' {
			pos.Line++
			pos.Character = 0
		} else if r > 0xffff {
			pos.Character += 2 // LSP uses UTF-16 code units.
		} else {
			pos.Character++
		}
	}
	active := []bool{true}
	for i := 0; i < len(runes); {
		start, first := i, pos
		r := runes[i]
		comment := false
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '{' || r == '(' && i+1 < len(runes) && runes[i+1] == '*':
			comment = true
			i++
			if r == '(' {
				i++
			}
			for i < len(runes) {
				if r == '{' && runes[i] == '}' {
					i++
					break
				}
				if r == '(' && runes[i] == '*' && i+1 < len(runes) && runes[i+1] == ')' {
					i += 2
					break
				}
				i++
			}
		case r == '/' && i+1 < len(runes) && runes[i+1] == '/':
			comment = true
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		case r == '\'':
			i++
			for i < len(runes) {
				if runes[i] == '\'' {
					i++
					if i < len(runes) && runes[i] == '\'' {
						i++
						continue
					}
					break
				}
				i++
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '&' || r == '$':
			i++
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
		default:
			i++
			if i < len(runes) && (r == ':' && runes[i] == '=' || r == '<' && (runes[i] == '=' || runes[i] == '>') || r == '>' && runes[i] == '=' || r == '.' && runes[i] == '.') {
				i++
			}
		}
		for _, consumed := range runes[start:i] {
			advance(consumed)
		}
		word := strings.ToLower(string(runes[start:i]))
		if comment {
			directive := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(word, "{"), "(*"), "}"), "*)"))
			fields := strings.Fields(directive)
			if len(fields) > 0 {
				switch fields[0] {
				case "$ifdef", "$ifndef", "$if", "$ifopt":
					active = append(active, active[len(active)-1])
				case "$else", "$elseif":
					if len(active) > 1 {
						active[len(active)-1] = false
					}
				case "$endif", "$ifend":
					if len(active) > 1 {
						active = active[:len(active)-1]
					}
				}
			}
			continue
		}
		if !unicode.IsSpace(r) && active[len(active)-1] {
			tokens = append(tokens, syntaxToken{word, Range{Start: first, End: pos}})
		}
	}
	return tokens
}

type statementParser struct {
	tokens      []syntaxToken
	i           int
	diagnostics []Diagnostic
}

func semicolonDiagnostics(source string) []Diagnostic {
	p := statementParser{tokens: syntaxTokens(source)}
	// Declaration parsing stays with the symbol indexer. Inspect executable
	// blocks, including program bodies and unit initialization/finalization.
	for p.i < len(p.tokens) {
		switch p.peek() {
		case "begin":
			p.statement()
		case "initialization", "finalization":
			p.i++
			p.sequence("finalization", "end")
		case "asm":
			p.statement()
		default:
			p.i++
		}
	}
	return p.diagnostics
}

func (p *statementParser) peek() string {
	if p.i >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.i].text
}

func (p *statementParser) take(word string) bool {
	if p.peek() != word {
		return false
	}
	p.i++
	return true
}

func syntaxOneOf(word string, words ...string) bool {
	for _, candidate := range words {
		if word == candidate {
			return true
		}
	}
	return false
}

func (p *statementParser) missingSemicolon() {
	if p.i == 0 {
		return
	}
	previous := p.tokens[p.i-1]
	p.diagnostics = append(p.diagnostics, Diagnostic{Range: previous.span, Severity: 1, Source: "delphi-lsp", Message: "Missing ';' between statements"})
}

func (p *statementParser) sequence(stops ...string) {
	for p.peek() != "" && !syntaxOneOf(p.peek(), stops...) {
		if p.take(";") {
			continue
		}
		start := p.i
		p.statement()
		if p.i == start {
			p.i++
			continue
		}
		if p.peek() != "" && !syntaxOneOf(p.peek(), stops...) && p.peek() != ";" {
			p.missingSemicolon()
		}
	}
}

func (p *statementParser) statement() {
	switch p.peek() {
	case "", ";", "end", "else", "until", "except", "finally":
		return
	case "begin":
		p.i++
		p.sequence("end")
		p.take("end")
	case "if":
		p.i++
		p.expression()
		if p.take("then") {
			p.statement()
		}
		if p.take("else") {
			p.statement()
		}
	case "while", "with", "for":
		p.i++
		for p.peek() != "" && !syntaxOneOf(p.peek(), "do", ";", "end") {
			p.i++
		}
		if p.take("do") {
			p.statement()
		}
	case "repeat":
		p.i++
		p.sequence("until")
		if p.take("until") {
			p.expression()
		}
	case "try":
		p.i++
		p.sequence("except", "finally", "end")
		if p.take("finally") {
			p.sequence("end")
		}
		if p.take("except") {
			p.sequence("else", "end")
			if p.take("else") {
				p.sequence("end")
			}
		}
		p.take("end")
	case "on":
		p.i++
		for p.peek() != "" && !syntaxOneOf(p.peek(), "do", ";", "end") {
			p.i++
		}
		if p.take("do") {
			p.statement()
		}
	case "case":
		p.i++
		p.expression()
		if !p.take("of") {
			return
		}
		for p.peek() != "" && !syntaxOneOf(p.peek(), "end", "else") {
			if p.take(";") {
				continue
			}
			for p.peek() != "" && !syntaxOneOf(p.peek(), ":", "end", "else") {
				p.i++
			}
			if !p.take(":") {
				break
			}
			p.statement()
			if !syntaxOneOf(p.peek(), "", ";", "end", "else") {
				p.missingSemicolon()
			}
		}
		if p.take("else") {
			p.sequence("end")
		}
		p.take("end")
	case "asm":
		p.i++
		for p.peek() != "" && p.peek() != "end" {
			p.i++
		}
		p.take("end")
	case "raise":
		p.i++
		p.expression()
		if p.take("at") {
			p.expression()
		}
	case "goto":
		p.i++
		p.atom()
	case "var", "const":
		p.i++
		for p.peek() != "" && !syntaxOneOf(p.peek(), ":=", "=", ";", "end") {
			p.i++
		}
		if p.take(":=") || p.take("=") {
			p.expression()
		}
	default:
		p.expression()
		if p.take(":") {
			p.statement()
			return
		} // Label.
		if p.take(":=") {
			p.expression()
		}
	}
}

// Only expression boundaries matter here; operator precedence has no effect
// on where a separator is required. Balanced arguments protect commas and
// semicolons in calls and anonymous method signatures.
func (p *statementParser) expression() {
	if !p.atom() {
		return
	}
	for syntaxOneOf(p.peek(), "+", "-", "*", "/", "=", "<>", "<", ">", "<=", ">=", "and", "or", "xor", "div", "mod", "shl", "shr", "as", "is", "in", "..") {
		p.i++
		if !p.atom() {
			return
		}
	}
}

func (p *statementParser) atom() bool {
	for syntaxOneOf(p.peek(), "+", "-", "not", "@", "inherited") {
		p.i++
	}
	word := p.peek()
	if syntaxOneOf(word, "", ";", "end", "else", "until", "then", "do", "of", "except", "finally", "begin", "if", "while", "for", "repeat", "try", "case", ")", "]", ",", ":", ":=", "at") {
		return false
	}
	if word == "procedure" || word == "function" {
		p.i++
		for p.peek() != "" && p.peek() != "begin" {
			p.i++
		}
		p.statement()
	} else if word == "#" {
		p.i++
		if p.peek() != "" {
			p.i++
		}
	} else if word == "(" || word == "[" {
		p.group()
	} else {
		p.i++
	}
	for {
		// Delphi concatenates quoted text and character codes without '+'.
		if strings.HasPrefix(p.peek(), "'") && (strings.HasPrefix(word, "'") || word == "#") {
			p.i++
			continue
		}
		switch p.peek() {
		case "(", "[":
			p.group()
		case ".":
			p.i++
			if p.peek() != "" {
				p.i++
			}
		case "^":
			p.i++
		case "#":
			p.i++
			if p.peek() != "" {
				p.i++
			}
		default:
			return true
		}
	}
}

func (p *statementParser) group() {
	close := ")"
	if p.peek() == "[" {
		close = "]"
	}
	p.i++
	for p.peek() != "" && p.peek() != close {
		if syntaxOneOf(p.peek(), "(", "[") {
			p.group()
		} else if p.peek() == "begin" {
			p.statement()
		} else {
			p.i++
		}
	}
	p.take(close)
}
