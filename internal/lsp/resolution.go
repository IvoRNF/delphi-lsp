package lsp

import "strings"

func (s *Server) definitionLocations(current *Document, position Position, name string) []Location {
	routine := routineAt(current, position)
	if routine != nil {
		var locals []Location
		for _, symbol := range current.Symbols {
			if strings.EqualFold(symbol.Name, name) && strings.EqualFold(symbol.Owner, routine.Name) {
				locals = append(locals, Location{URI: current.URI, Range: symbol.Selection})
			}
		}
		if len(locals) > 0 {
			return locals
		}
	}
	owner := memberOwnerAt(current, position, routine)
	if owner != "" {
		var members []Location
		for _, symbol := range current.Symbols {
			if symbol.Kind != symbolFunction && strings.EqualFold(symbol.Name, name) && strings.EqualFold(symbol.Owner, owner) {
				members = append(members, Location{URI: current.URI, Range: symbol.Selection})
			}
		}
		if len(members) > 0 {
			return members
		}
	}
	var matches []Location
	for uri, document := range s.docs {
		for _, symbol := range document.Symbols {
			if symbol.Owner == "" && strings.EqualFold(symbol.Name, name) {
				matches = append(matches, Location{URI: uri, Range: symbol.Selection})
			}
		}
	}
	return matches
}

func routineAt(document *Document, position Position) *Symbol {
	for index := range document.Symbols {
		symbol := &document.Symbols[index]
		if symbol.Kind == symbolFunction && position.Line >= symbol.Scope.Start.Line && position.Line <= symbol.Scope.End.Line {
			return symbol
		}
	}
	return nil
}

func memberOwnerAt(document *Document, position Position, routine *Symbol) string {
	if routine != nil && routine.Owner != "" {
		return routine.Owner
	}
	for _, symbol := range document.Symbols {
		if symbol.Kind == symbolClass && position.Line >= symbol.Scope.Start.Line && position.Line <= symbol.Scope.End.Line {
			return symbol.Name
		}
	}
	return ""
}
