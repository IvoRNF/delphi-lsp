package lsp

import "strings"

type parameterUse struct {
	token syntaxToken
	used  bool
}

type parameterScope struct {
	parent   *parameterScope
	bindings map[string]*parameterUse // A nil binding shadows an outer parameter.
}

func newParameterScope(parent *parameterScope) *parameterScope {
	return &parameterScope{parent: parent, bindings: map[string]*parameterUse{}}
}

func parameterName(name string) string { return strings.TrimPrefix(name, "&") }

func (s *parameterScope) use(name string) {
	for ; s != nil; s = s.parent {
		if binding, ok := s.bindings[parameterName(name)]; ok {
			if binding != nil {
				binding.used = true
			}
			return
		}
	}
}

// Assembly and implicit inherited calls can use arguments without naming them.
func (s *parameterScope) useAll() {
	for ; s != nil; s = s.parent {
		for _, binding := range s.bindings {
			if binding != nil {
				binding.used = true
			}
		}
	}
}

type unusedParameterParser struct {
	unitParser
	warnings []Diagnostic
}

func unusedParameterTokenDiagnostics(tokens []syntaxToken) []Diagnostic {
	p := unusedParameterParser{unitParser: unitParser{tokens: tokens}}
	p.declarations(nil, false)
	return p.warnings
}

// Parse declarations separately from bodies: prototypes, procedural types and
// member declarations must never borrow the body of a later implementation.
func (p *unusedParameterParser) declarations(scope *parameterScope, routine bool) bool {
	section, declarationKind := "", ""
	for p.word(0) != "" {
		switch p.word(0) {
		case "interface", "implementation", "initialization", "finalization":
			if routine {
				return false
			}
			section, declarationKind = p.word(0), ""
			p.i++
		case "begin", "asm":
			complete := p.body(scope)
			if routine {
				return complete
			}
		case "end":
			return false
		case "forward", "external", "abstract":
			p.skipDeclaration(nil)
			if routine {
				return false
			}
		case "var", "threadvar", "const", "resourcestring", "type", "label":
			declarationKind = p.word(0)
			p.i++
		case "procedure", "function", "constructor", "destructor", "operator":
			declarationKind = ""
			p.routine(scope, section != "interface")
		default:
			if declarationKind != "" && syntaxIdentifier(p.word(0)) {
				// Local declarations hide parameters of enclosing routines.
				if scope != nil {
					scope.bindings[parameterName(p.word(0))] = nil
					for p.word(1) == "," && syntaxIdentifier(p.word(2)) {
						p.i += 2
						scope.bindings[parameterName(p.word(0))] = nil
					}
				}
				p.skipDeclaration(scope)
			} else {
				p.i++
			}
		}
	}
	return false
}

func (p *unusedParameterParser) skipDeclaration(scope *parameterScope) {
	initializer := false
	for !syntaxOneOf(p.word(0), "", ";", "begin", "end", "implementation") {
		switch p.word(0) {
		case "(", "[", "<":
			if !initializer {
				p.skipVariableGroup()
				continue
			}
		case "record", "object", "interface", "dispinterface", "class":
			if p.word(-1) != "of" && !syntaxOneOf(p.word(1), ";", "of") && !p.abbreviatedClass() {
				p.skipVariableRecord()
				continue
			}
		case "=", ":=", "absolute":
			initializer = true
		}
		if initializer {
			p.reference(scope)
		}
		p.i++
	}
	if p.word(0) == ";" {
		p.i++
	}
}

func (p *unusedParameterParser) routine(parent *parameterScope, implementation bool) {
	p.i++
	scope := newParameterScope(parent)
	// Anonymous routines start directly with '(' / ':' / 'begin'.
	if syntaxIdentifier(p.word(0)) && p.word(0) != "asm" {
		name := parameterName(p.word(0))
		if parent != nil {
			parent.bindings[name] = nil
		}
		for syntaxIdentifier(p.word(0)) || p.word(0) == "." || p.word(0) == "<" {
			if p.word(0) == "<" {
				p.skipVariableGroup()
			} else {
				name = parameterName(p.word(0))
				p.i++
			}
		}
		scope.bindings[name] = nil // Assignment to a function's own name.
	}
	var parameters []*parameterUse
	errorsBefore := len(p.diagnostics)
	if p.word(0) == "(" {
		for _, token := range p.parameters() {
			parameter := &parameterUse{token: token}
			parameters = append(parameters, parameter)
			scope.bindings[parameterName(token.text)] = parameter
		}
		if p.word(-1) != ")" {
			return
		}
	}
	// Skip the return type. Named routines end their header with a semicolon;
	// anonymous routines proceed directly to their declarations/body.
	if p.word(0) == ":" {
		p.skipDeclaration(nil)
	} else if p.word(0) == ";" {
		p.i++
	}
	if !implementation {
		return
	}
	warningsBefore := len(p.warnings)
	complete := p.declarations(scope, true)
	if !complete || len(p.diagnostics) != errorsBefore {
		// Do not produce speculative warnings while the user is typing a body.
		p.warnings = p.warnings[:warningsBefore]
		return
	}
	for _, parameter := range parameters {
		if !parameter.used {
			p.warnings = append(p.warnings, Diagnostic{
				Range: parameter.token.span, Severity: 2, Source: "delphi-lsp",
				Message: "Unused parameter '" + parameter.token.text + "'", Tags: []int{1},
			})
		}
	}
}

func (p *unusedParameterParser) reference(scope *parameterScope) {
	if scope != nil && p.word(-1) != "." && syntaxIdentifier(p.word(0)) && !strings.HasPrefix(p.word(0), "$") {
		scope.use(p.word(0))
	}
}

// Walk balanced executable blocks, preserving lexical scopes for nested and
// anonymous routines. Reads, writes and passing an argument all count as use.
func (p *unusedParameterParser) body(scope *parameterScope) bool {
	assembly := p.word(0) == "asm"
	if assembly {
		scope.useAll()
	}
	p.i++
	for p.word(0) != "" {
		if p.word(0) == "end" {
			p.i++
			return true
		}
		if assembly {
			p.i++
			continue
		}
		switch p.word(0) {
		case "implementation", "initialization", "finalization":
			return false
		case "begin", "try", "case", "asm":
			if !p.body(newParameterScope(scope)) {
				return false
			}
		case "procedure", "function":
			p.routine(scope, true)
		case "var", "const":
			p.i++
			if scope != nil && syntaxIdentifier(p.word(0)) {
				scope.bindings[parameterName(p.word(0))] = nil
			}
			p.skipDeclaration(scope)
		default:
			if p.word(0) == "inherited" && syntaxOneOf(p.word(1), ";", "end", "else") {
				scope.useAll()
			}
			p.reference(scope)
			p.i++
		}
	}
	return false
}
