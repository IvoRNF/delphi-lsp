package lsp

import "strings"

// resultWarningPrefix identifies "function never assigns Result" warnings so
// resultTokenDiagnostics can return them apart from the other declaration
// warnings produced by the shared routine walker.
const resultWarningPrefix = "Function '"

// resultCheck tracks one routine while its body is walked. Only named functions
// are reported; procedures and anonymous routines still receive a frame so that
// a nested Result assignment is attributed to the function that owns it.
type resultCheck struct {
	name       string
	isFunction bool
	report     bool
	assigned   bool
	assembly   bool
	bodyStart  int
	bodyEnd    int
	token      syntaxToken
}

// resultTokenDiagnostics reports named functions whose body never assigns the
// implicit Result variable. It follows Delphi's rules: Result :=, assignment to
// the function's own name and Exit(value) all set the result, an assembler body
// may set the result register, and a body whose last statement always raises
// never falls through to an undefined result.
func resultTokenDiagnostics(tokens []syntaxToken) []Diagnostic {
	return unusedDiagnosticsByPrefix(unusedDeclarationTokenDiagnostics(tokens), resultWarningPrefix)
}

func (p *unusedParameterParser) currentResultFrame() *resultCheck {
	if len(p.resultStack) == 0 {
		return nil
	}
	return p.resultStack[len(p.resultStack)-1]
}

func (p *unusedParameterParser) pushResultFrame(keyword, name string, nameToken syntaxToken, implementation bool) *resultCheck {
	frame := &resultCheck{name: name, isFunction: keyword == "function", token: nameToken, bodyStart: -1}
	frame.report = implementation && frame.isFunction && name != ""
	p.resultStack = append(p.resultStack, frame)
	return frame
}

func (p *unusedParameterParser) popResultFrame() {
	if len(p.resultStack) > 0 {
		p.resultStack = p.resultStack[:len(p.resultStack)-1]
	}
}

// trackResultUse is called for every identifier while a routine body is walked.
// It recognises the ways Delphi sets a function result: Result :=, assignment to
// the function's own name, Exit(value), plus member writes and arguments of a
// routine call (SetLength(Result, N) and similar var-parameter idioms).
func (p *unusedParameterParser) trackResultUse() {
	if len(p.resultStack) == 0 || p.word(-1) == "." {
		return
	}
	name := parameterName(p.word(0))
	if name == "" {
		return
	}
	previous, next := p.word(-1), p.word(1)
	if strings.EqualFold(name, "result") {
		switch {
		case syntaxOneOf(next, ":=", ".", "[", "^"):
			p.markResultAssigned("")
		case syntaxOneOf(previous, "(", ",") && syntaxOneOf(next, ",", ")", ";"):
			p.markResultAssigned("")
		}
		return
	}
	if strings.EqualFold(name, "exit") && next == "(" {
		p.markResultAssigned("")
		return
	}
	switch {
	case next == ":=":
		p.markResultAssigned(name)
	case syntaxOneOf(previous, "(", ",") && syntaxOneOf(next, ",", ")", ";"):
		// A function's own name passed to a routine, e.g. Load(F).
		p.markResultAssigned(name)
	}
}

// markResultAssigned records an assignment. An empty name is Result, which
// belongs to the innermost function: a nested procedure shares its enclosing
// function's Result, while a nested function has its own. A non-empty name is an
// assignment to a function's own name, which may also target an enclosing
// function.
func (p *unusedParameterParser) markResultAssigned(name string) {
	for i := len(p.resultStack) - 1; i >= 0; i-- {
		frame := p.resultStack[i]
		if !frame.isFunction {
			continue
		}
		if name == "" {
			frame.assigned = true
			return
		}
		if strings.EqualFold(frame.name, name) {
			frame.assigned = true
			return
		}
	}
}

func (p *unusedParameterParser) reportResult(frame *resultCheck) {
	if !frame.report || frame.assigned || frame.assembly {
		return
	}
	if frame.bodyStart < 0 || frame.bodyEnd <= frame.bodyStart+1 {
		return
	}
	if bodyAlwaysRaises(p.tokens[frame.bodyStart+1 : frame.bodyEnd-1]) {
		return
	}
	p.warnings = append(p.warnings, Diagnostic{
		Range:    frame.token.span,
		Severity: 2,
		Source:   "delphi-lsp",
		Message:  resultWarningPrefix + frame.name + "' does not assign a value to 'Result'",
	})
}

// bodyAlwaysRaises reports whether a routine body cannot fall through to its
// closing end because its last top-level statement always raises. Top-level
// statements are the semicolon-separated segments; begin/try/case/asm/repeat
// blocks and parenthesized anonymous methods keep their inner semicolons nested.
// A literal begin ... end statement is unwrapped so its own last statement is
// inspected too. If/else branches that both raise are not analysed.
func bodyAlwaysRaises(tokens []syntaxToken) bool {
	depth, start, lastStart, lastEnd := 0, -1, -1, -1
	for i, token := range tokens {
		switch token.text {
		case "begin", "try", "case", "asm", "repeat", "(", "[":
			depth++
		case "end", "until", ")", "]":
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && token.text == ";" {
			if start >= 0 {
				lastStart, lastEnd = start, i
			}
			start = -1
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		lastStart, lastEnd = start, len(tokens)
	}
	if lastStart < 0 {
		return false
	}
	segment := tokens[lastStart:lastEnd]
	if len(segment) > 1 && segment[0].text == "begin" && segment[len(segment)-1].text == "end" {
		return bodyAlwaysRaises(segment[1 : len(segment)-1])
	}
	return segment[0].text == "raise"
}
