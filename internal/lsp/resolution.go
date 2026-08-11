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
	if local := localDefinitionLocations(current, name); len(local) > 0 {
		return local
	}
	var matches []Location
	for _, ref := range s.symbolRefs(name) {
		if ref.symbol.Owner == "" {
			matches = append(matches, Location{URI: ref.uri, Range: ref.symbol.Selection})
		}
	}
	return uniqueLocations(matches)
}

// localDefinitionLocations keeps a same-named declaration in the current
// unit ahead of identically named workspace symbols. For routines, the
// implementation is the useful navigation target.
func localDefinitionLocations(document *Document, name string) []Location {
	var implementations, declarations []Location
	for _, symbol := range document.Symbols {
		if symbol.Owner != "" || !strings.EqualFold(symbol.Name, name) {
			continue
		}
		location := Location{URI: document.URI, Range: symbol.Selection}
		if isRoutineSymbol(symbol) && symbol.Implementation {
			implementations = append(implementations, location)
		} else {
			declarations = append(declarations, location)
		}
	}
	if len(implementations) > 0 {
		return uniqueLocations(implementations)
	}
	return uniqueLocations(declarations)
}

func receiverAt(text string, position Position) string {
	lines := strings.Split(text, "\n")
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
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	return line[start:end]
}

func declaredTypeOf(document *Document, name string, routine *Symbol) string {
	owners := []string{""}
	if routine != nil {
		owners = []string{routine.Name, routine.Owner, ""}
	}
	for _, owner := range owners {
		for _, symbol := range document.Symbols {
			if !strings.EqualFold(symbol.Name, name) || !strings.EqualFold(symbol.Owner, owner) {
				continue
			}
			if symbol.Kind == symbolClass {
				return symbol.Name
			}
			if colon := strings.Index(symbol.Detail, ":"); colon >= 0 {
				parts := strings.Fields(strings.TrimSpace(symbol.Detail[colon+1:]))
				if len(parts) > 0 {
					return parts[0]
				}
			}
		}
	}
	return ""
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
	if uri := s.unitURI(name); uri != "" {
		s.ensureParsed(uri)
		if d := s.document(uri); d != nil {
			for i := range d.Symbols {
				sym := &d.Symbols[i]
				if sym.Kind == symbolModule && strings.EqualFold(sym.Name, name) {
					return &Location{URI: uri, Range: sym.Selection}
				}
			}
		}
	}
	for _, ref := range s.symbolRefs(name) {
		if ref.symbol.Kind == symbolModule {
			return &Location{URI: ref.uri, Range: ref.symbol.Selection}
		}
	}
	return nil
}

// routineImplementationAt redirects a declaration header to its implementation.
// It also covers class method declarations, whose symbol kind is symbolMethod.
func (s *Server) routineImplementationAt(current *Document, position Position, name string) *Location {
	for i := range current.Symbols {
		symbol := &current.Symbols[i]
		if !isRoutineSymbol(*symbol) || symbol.Implementation || !sameRangePosition(symbol.Selection, position) || !strings.EqualFold(symbol.Name, name) {
			continue
		}
		for _, ref := range s.symbolRefs(symbol.Name) {
			candidate := ref.symbol
			if candidate.Implementation && strings.EqualFold(candidate.Name, symbol.Name) && strings.EqualFold(candidate.Owner, symbol.Owner) {
				return &Location{URI: ref.uri, Range: candidate.Selection}
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
