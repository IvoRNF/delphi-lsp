package lsp

// Check classic local var sections separately from executable statements.
// A declaration needs its terminator even immediately before begin.
func localVariableDiagnostics(source string) []Diagnostic {
	p := unitParser{tokens: syntaxTokens(source)}
	routine := false
	inInterface := false
	for p.word(0) != "" {
		switch p.word(0) {
		case "interface":
			if p.word(-1) == "=" {
				if p.word(1) == ";" {
					p.i++
					continue
				}
				p.skipVariableRecord()
				continue
			}
			inInterface, routine = true, false
		case "implementation":
			inInterface, routine = false, false
		case "initialization", "finalization", "forward", "external":
			routine = false
		case "begin", "asm":
			body := statementParser{tokens: p.tokens, i: p.i}
			body.statement()
			p.i = body.i
			routine = false
			continue
		case "(", "[":
			p.skipVariableGroup()
			continue
		case "record", "object":
			if p.word(-1) != "of" {
				p.skipVariableRecord()
				continue
			}
		case "class":
			if p.word(-1) == "=" && !syntaxOneOf(p.word(1), ";", "of") && !p.abbreviatedClass() {
				p.skipVariableRecord()
				continue
			}
		case "procedure", "function", "constructor", "destructor":
			if !inInterface && !syntaxOneOf(p.word(-1), "=", "to") {
				routine = true
			}
		case "var":
			if routine {
				p.i++
				p.localVariables()
				continue
			}
		}
		p.i++
	}
	return p.diagnostics
}

// Recognize name[, name]* ':' without depending on line breaks.
func (p *unitParser) variableColon() int {
	offset := 0
	for syntaxIdentifier(p.word(offset)) {
		offset++
		if p.word(offset) == ":" {
			return offset
		}
		if p.word(offset) != "," {
			break
		}
		offset++
	}
	return -1
}

func (p *unitParser) localVariables() {
	for {
		colon := p.variableColon()
		if colon < 0 {
			return
		}
		p.i += colon + 1
		typeStart := p.i
		for p.word(0) != "" && p.word(0) != ";" {
			word := p.word(0)
			if syntaxOneOf(word, "begin", "asm", "var", "const", "type", "label", "resourcestring", "implementation", "initialization", "finalization", "end") {
				break
			}
			if p.i > typeStart && (p.variableColon() >= 0 || syntaxOneOf(word, "procedure", "function", "constructor", "destructor") && p.word(-1) != "to") {
				break
			}
			if word == "(" || word == "[" || word == "<" {
				p.skipVariableGroup()
				continue
			}
			if word == "record" {
				p.skipVariableRecord()
				continue
			}
			p.i++
		}
		if p.word(0) == ";" {
			p.i++
			continue
		}
		if p.i > typeStart {
			p.diagnostics = append(p.diagnostics, Diagnostic{
				Range: p.tokens[p.i-1].span, Severity: 1, Source: "delphi-lsp",
				Message: "Missing ';' after local variable declaration",
			})
		}
		if p.i == typeStart {
			return
		}
	}
}

func (p *unitParser) skipVariableGroup() {
	close := map[string]string{"(": ")", "[": "]", "<": ">"}[p.word(0)]
	p.i++
	for p.word(0) != "" && p.word(0) != close {
		if syntaxOneOf(p.word(0), "(", "[", "<") {
			p.skipVariableGroup()
		} else {
			p.i++
		}
	}
	if p.word(0) == close {
		p.i++
	}
}

func (p *unitParser) skipVariableRecord() {
	p.i++
	for p.word(0) != "" && p.word(0) != "end" {
		if syntaxOneOf(p.word(0), "record", "object") && p.word(-1) != "of" {
			p.skipVariableRecord()
		} else {
			p.i++
		}
	}
	if p.word(0) == "end" {
		p.i++
	}
}
