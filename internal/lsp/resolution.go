package lsp

import (
	"path/filepath"
	"strconv"
	"strings"
)

func (s *Server) definitionLocations(current *Document, position Position, name string) []Location {
	if unit := current.useAt(position); unit != nil {
		if location := s.unitLocation(unit.Name); location != nil {
			return []Location{*location}
		}
	}
	if implementation := s.routineImplementationAt(current, position, name); implementation != nil {
		return []Location{*implementation}
	}
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
	return uniqueLocations(matches)
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

func (d *Document) useAt(position Position) *UnitReference {
	for index := range d.Uses {
		unit := &d.Uses[index]
		if position.Line == unit.Range.Start.Line && position.Character >= unit.Range.Start.Character && position.Character <= unit.Range.End.Character {
			return unit
		}
	}
	return nil
}

func (s *Server) unitLocation(name string) *Location {
	for uri, document := range s.docs {
		for _, symbol := range document.Symbols {
			if symbol.Kind == symbolModule && strings.EqualFold(symbol.Name, name) {
				location := Location{URI: uri, Range: symbol.Selection}
				return &location
			}
		}
	}
	return nil
}

// routineImplementationAt redirects a declaration header to its implementation.
// It also covers class method declarations, whose symbol kind is symbolMethod.
func (s *Server) routineImplementationAt(current *Document, position Position, name string) *Location {
	for _, symbol := range current.Symbols {
		if !isRoutineSymbol(symbol) || symbol.Implementation || !sameRangePosition(symbol.Selection, position) || !strings.EqualFold(symbol.Name, name) {
			continue
		}
		for uri, document := range s.docs {
			for _, candidate := range document.Symbols {
				if candidate.Implementation && strings.EqualFold(candidate.Name, symbol.Name) && strings.EqualFold(candidate.Owner, symbol.Owner) {
					location := Location{URI: uri, Range: candidate.Selection}
					return &location
				}
			}
		}
	}
	return nil
}

func isRoutineSymbol(symbol Symbol) bool {
	return symbol.Kind == symbolFunction || symbol.Kind == symbolMethod
}

func sameRangePosition(r Range, position Position) bool {
	return position.Line == r.Start.Line && position.Character >= r.Start.Character && position.Character <= r.End.Character
}

func uniqueLocations(locations []Location) []Location {
	seen := map[string]bool{}
	unique := make([]Location, 0, len(locations))
	for _, location := range locations {
		key := canonicalURI(location.URI) + "|" + positionKey(location.Range.Start) + "|" + positionKey(location.Range.End)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, location)
		}
	}
	return unique
}

func canonicalURI(raw string) string {
	if path := uriPath(raw); path != "" {
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		return strings.ToLower(filepath.Clean(path))
	}
	return strings.ToLower(raw)
}

func positionKey(position Position) string {
	return strconv.Itoa(position.Line) + ":" + strconv.Itoa(position.Character)
}
