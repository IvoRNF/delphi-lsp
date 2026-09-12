package lsp

import (
	"strings"
	"unicode"
)

// Validate the unit envelope without treating type interfaces or nested ends
// as unit sections. This pass deliberately leaves declaration semantics to a
// future compiler-aware parser.
func unitDiagnostics(uri, source string) []Diagnostic {
	tokens := syntaxTokens(source)
	if len(tokens) > 0 && syntaxOneOf(tokens[0].text, "program", "library", "package") {
		return nil
	}
	if (len(tokens) == 0 || tokens[0].text != "unit") && !strings.HasSuffix(strings.ToLower(uri), ".pas") {
		return nil
	}
	p := unitParser{tokens: tokens}
	p.parse()
	return p.diagnostics
}

type unitParser struct {
	tokens      []syntaxToken
	i           int
	diagnostics []Diagnostic
}

func (p *unitParser) word(offset int) string {
	i := p.i + offset
	if i < 0 || i >= len(p.tokens) {
		return ""
	}
	return p.tokens[i].text
}

func (p *unitParser) report(message string) {
	span := Range{}
	if p.i < len(p.tokens) {
		span = p.tokens[p.i].span
	} else if len(p.tokens) > 0 {
		span.Start = p.tokens[len(p.tokens)-1].span.End
		span.End = span.Start
	}
	p.diagnostics = append(p.diagnostics, Diagnostic{Range: span, Severity: 1, Source: "delphi-lsp", Message: message})
}

func syntaxIdentifier(word string) bool {
	if strings.HasPrefix(word, "&") {
		word = word[1:]
	} else if syntaxOneOf(word,
		"unit", "interface", "implementation", "initialization", "finalization", "end", "begin", "uses", "type", "const", "var", "procedure", "function", "class", "record", "program", "library", "package", "in", "of", "then", "else", "do", "if", "for", "while", "try", "except", "finally", "repeat", "until") {
		return false
	}
	for i, r := range word {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return word != ""
}

func (p *unitParser) name() bool {
	if !syntaxIdentifier(p.word(0)) {
		p.report("Expected unit identifier")
		return false
	}
	p.i++
	for p.word(0) == "." {
		p.i++
		if !syntaxIdentifier(p.word(0)) {
			p.report("Expected identifier after '.'")
			return false
		}
		p.i++
	}
	return true
}

func (p *unitParser) uses() {
	p.i++
	for {
		if !p.name() {
			break
		}
		if p.word(0) == "in" {
			p.i++
			if !strings.HasPrefix(p.word(0), "'") {
				p.report("Expected file name string after 'in'")
			} else {
				p.i++
			}
		}
		if p.word(0) != "," {
			break
		}
		p.i++
	}
	if p.word(0) == ";" {
		p.i++
	} else {
		p.report("Expected ';' after uses clause")
	}
}

type unitBlock struct {
	word  string
	index int
}

// An empty descendant may omit end: TError = class(Exception);
// Look beyond the ancestor list without consuming it, so ordinary classes
// with members still require their closing end.
func (p *unitParser) abbreviatedClass() bool {
	offset := 1
	for syntaxOneOf(p.word(offset), "abstract", "sealed") {
		offset++
	}
	if p.word(offset) != "(" {
		return false
	}
	depth := 0
	for ; p.word(offset) != ""; offset++ {
		switch p.word(offset) {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return p.word(offset+1) == ";"
			}
		case ";", "implementation", "end":
			return false
		}
	}
	return false
}

func (p *unitParser) parse() {
	if p.word(0) == "unit" {
		p.i++
	} else {
		p.report("Expected 'unit' heading")
	}
	if p.word(0) != "interface" && p.word(0) != "implementation" {
		p.name()
		for syntaxOneOf(p.word(0), "deprecated", "platform", "library", "experimental") {
			p.i++
			if strings.HasPrefix(p.word(0), "'") {
				p.i++
			}
		}
		if p.word(0) == ";" {
			p.i++
		} else {
			p.report("Expected ';' after unit heading")
		}
	}
	seen := map[string]bool{}
	if p.word(0) != "interface" {
		p.report("Expected 'interface' after unit heading")
	}
	section := ""
	pendingRoutines := 0
	parentheses := 0
	usesAllowed := false
	var blocks []unitBlock
	closed := false
	for p.i < len(p.tokens) {
		word := p.word(0)
		// A type named interface follows '='; a section keyword does not.
		typeInterface := word == "interface" && p.word(-1) == "="
		if !typeInterface && syntaxOneOf(word, "interface", "implementation", "initialization", "finalization") {
			if len(blocks) > 0 {
				p.report("Unclosed block before '" + word + "' section")
				blocks = nil // Recover at the next unambiguous section boundary.
			}
			if seen[word] {
				p.report("Duplicate '" + word + "' section")
			}
			switch word {
			case "interface":
				if section != "" {
					p.report("'interface' must precede other unit sections")
				}
			case "implementation":
				if !seen["interface"] {
					p.report("Expected 'interface' before 'implementation'")
				}
				if syntaxOneOf(section, "initialization", "finalization") {
					p.report("'implementation' must precede initialization and finalization")
				}
			case "initialization":
				if !seen["implementation"] {
					p.report("Expected 'implementation' before 'initialization'")
				}
				if seen["finalization"] {
					p.report("'initialization' must precede 'finalization'")
				}
			case "finalization":
				if !seen["initialization"] {
					p.report("'finalization' requires an 'initialization' section")
				}
			}
			seen[word], section = true, word
			usesAllowed = syntaxOneOf(word, "interface", "implementation")
			p.i++
			continue
		}
		if len(blocks) == 0 && word == "uses" {
			if !usesAllowed {
				p.report("'uses' must appear immediately after 'interface' or 'implementation'")
			}
			usesAllowed = false
			p.uses()
			continue
		}
		usesAllowed = false
		if word == "(" {
			parentheses++
		}
		if word == ")" && parentheses > 0 {
			parentheses--
		}
		if len(blocks) == 0 && parentheses == 0 && section == "implementation" {
			if syntaxOneOf(word, "procedure", "function", "constructor", "destructor", "operator") && !syntaxOneOf(p.word(-1), "=", "to") {
				pendingRoutines++
			}
			if syntaxOneOf(word, "forward", "external") && pendingRoutines > 0 {
				pendingRoutines--
			}
		}
		if word == "end" && len(blocks) == 0 {
			p.i++
			if p.word(0) != "." {
				p.report("Expected '.' after unit 'end'")
				if p.word(0) == ";" {
					p.i++
				}
			} else {
				p.i++
			}
			closed = true
			break
		}
		push := syntaxOneOf(word, "begin", "try", "repeat", "asm")
		if word == "case" {
			// A variant record's case shares the record's closing end.
			push = len(blocks) == 0 || blocks[len(blocks)-1].word != "record"
		}
		if word == "record" && p.word(-1) != ":" || word == "object" && p.word(-1) != "of" || typeInterface || word == "dispinterface" {
			push = true
		}
		if word == "class" && !syntaxOneOf(p.word(1), "of", ";", "procedure", "function", "constructor", "destructor", "operator", "var", "property") {
			// Generic constraints (T: class) do not introduce a class body.
			push = p.word(-1) == "=" && !p.abbreviatedClass()
		}
		if (typeInterface || word == "dispinterface") && p.word(1) == ";" {
			push = false
		}
		if push {
			if word == "begin" && section == "interface" {
				p.report("Routine bodies are not allowed in the interface section")
			}
			blockWord := word
			if syntaxOneOf(word, "begin", "asm") && len(blocks) == 0 && pendingRoutines > 0 {
				blockWord = "routine"
				pendingRoutines--
			}
			blocks = append(blocks, unitBlock{word: blockWord, index: p.i})
			if word == "asm" {
				p.i++
				for p.word(0) != "" && p.word(0) != "end" {
					p.i++
				}
				continue
			}
		} else if word == "end" || word == "until" {
			if len(blocks) == 0 {
				p.report("Unexpected '" + word + "'")
			} else {
				block := blocks[len(blocks)-1]
				if (block.word == "repeat") != (word == "until") {
					p.report("Mismatched block terminator '" + word + "'")
				}
				blocks = blocks[:len(blocks)-1]
				if block.word == "routine" && p.word(1) != ";" {
					p.report("Expected ';' after routine body")
				}
				// Delphi also supports a legacy begin ... end. initializer.
				if word == "end" && p.word(1) == "." && len(blocks) == 0 && block.word == "begin" && section == "implementation" {
					p.i += 2
					closed = true
					break
				}
			}
		}
		p.i++
	}
	if closed && p.i < len(p.tokens) {
		p.report("Unexpected content after unit 'end.'")
	}
	for _, block := range blocks {
		p.diagnostics = append(p.diagnostics, Diagnostic{Range: p.tokens[block.index].span, Severity: 1, Source: "delphi-lsp", Message: "Unclosed '" + block.word + "' block"})
	}
	if !seen["interface"] {
		p.report("Missing 'interface' section")
	}
	if !seen["implementation"] {
		p.report("Missing 'implementation' section")
	}
	if !closed {
		p.report("Expected 'end.' to close unit")
	}
}
