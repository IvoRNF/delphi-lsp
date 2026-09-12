package lsp

func parameterDiagnostics(source string) []Diagnostic {
	p := unitParser{tokens: syntaxTokens(source)}
	for p.word(0) != "" {
		if !syntaxOneOf(p.word(0), "procedure", "function", "constructor", "destructor", "operator") {
			p.i++
			continue
		}
		p.i++
		// Skip a qualified routine name and generic parameters, but never
		// cross a declaration boundary looking for an argument list.
		for syntaxIdentifier(p.word(0)) || p.word(0) == "." || p.word(0) == "<" {
			if p.word(0) == "<" {
				p.skipVariableGroup()
			} else {
				p.i++
			}
		}
		if p.word(0) == "(" {
			p.parameters()
		}
	}
	return p.diagnostics
}

func parameterModifier(word string) bool {
	return syntaxOneOf(word, "const", "var", "out", "constref")
}

func (p *unitParser) parameterSeparator(message string) {
	span := p.tokens[p.i].span
	if p.i > 0 {
		span = p.tokens[p.i-1].span
	}
	p.diagnostics = append(p.diagnostics, Diagnostic{Range: span, Severity: 1, Source: "delphi-lsp", Message: message})
}

func (p *unitParser) parameters() {
	p.i++
	for p.word(0) != "" && p.word(0) != ")" {
		start := p.i
		for p.word(0) == "[" {
			p.skipVariableGroup()
		} // Parameter attributes.
		if parameterModifier(p.word(0)) {
			p.i++
		}
		// Names in one group use commas: A, B: Integer.
		if !syntaxIdentifier(p.word(0)) {
			return
		}
		p.i++
		for p.word(0) == "," {
			p.i++
			if !syntaxIdentifier(p.word(0)) {
				return
			}
			p.i++
		}
		if p.word(0) == ":" {
			p.i++
			typeStart := p.i
			for !syntaxOneOf(p.word(0), "", ";", ")") {
				word := p.word(0)
				if syntaxOneOf(word, "begin", "end", "implementation") {
					return
				}
				if p.i > typeStart && (p.variableColon() >= 0 || parameterModifier(word) && !(word == "const" && p.word(-1) == "of")) {
					break
				}
				if word == "," {
					p.report("Expected ';' instead of ',' between parameter groups")
					p.i++
					break
				}
				if syntaxOneOf(word, "procedure", "function") {
					p.i++
					if p.word(0) == "(" {
						p.parameters()
					}
					continue
				}
				if syntaxOneOf(word, "(", "[", "<") {
					p.skipVariableGroup()
				} else {
					p.i++
				}
			}
		}
		if p.word(0) == ";" {
			p.i++
		} else if p.word(0) != ")" && p.word(0) != "" {
			if p.word(-1) != "," {
				p.parameterSeparator("Missing ';' between parameter groups")
			}
		}
		if p.i == start {
			return
		}
	}
	if p.word(0) == ")" {
		p.i++
	}
}
