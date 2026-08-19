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
		// Parameters belong to the active routine header. This keeps an
		// interface declaration's parameters out of an implementation lookup.
		var parameters []Location
		var locals []Location
		for _, symbol := range current.Symbols {
			if strings.EqualFold(symbol.Name, name) && strings.EqualFold(symbol.Owner, routine.Name) {
				location := Location{URI: current.URI, Range: symbol.Selection}
				if symbol.Selection.Start.Line == routine.Selection.Start.Line {
					parameters = append(parameters, location)
				} else if symbol.Selection.Start.Line >= routine.Scope.Start.Line && symbol.Selection.Start.Line <= routine.Scope.End.Line {
					locals = append(locals, location)
				}
			}
		}
		if len(locals) > 0 {
			return uniqueLocations(locals)
		}
		if len(parameters) > 0 {
			return uniqueLocations(parameters)
		}
	}
	if typeName := s.memberTypeAt(current, position); typeName != "" {
		if members := s.memberDefinitionLocations(current, typeName, name); len(members) > 0 {
			return members
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
	if local := localDefinitionLocations(current, position, name); len(local) > 0 {
		return local
	}
	return locationsForRefs(s.visibleSymbolRefs(current, name), lookupIntentAt(current.Lines, position))
}

// localDefinitionLocations keeps a same-named declaration in the current
// unit ahead of identically named workspace symbols. For routines, the
// implementation is the useful navigation target.
func localDefinitionLocations(document *Document, position Position, name string) []Location {
	var implementations, declarations, values []Location
	for _, symbol := range document.Symbols {
		if symbol.Owner != "" || !strings.EqualFold(symbol.Name, name) {
			continue
		}
		location := Location{URI: document.URI, Range: symbol.Selection}
		if isRoutineSymbol(symbol) && symbol.Implementation {
			implementations = append(implementations, location)
		} else if !isRoutineSymbol(symbol) {
			values = append(values, location)
		} else {
			declarations = append(declarations, location)
		}
	}
	if lookupIntentAt(document.Lines, position) == lookupValue && len(values) > 0 {
		return uniqueLocations(values)
	}
	if len(implementations) > 0 {
		return uniqueLocations(implementations)
	}
	if len(declarations) > 0 {
		return uniqueLocations(declarations)
	}
	return uniqueLocations(values)
}

type lookupIntent int

const (
	lookupUnknown lookupIntent = iota
	lookupRoutine
	lookupValue
)

// lookupIntentAt distinguishes the two unambiguous forms that matter for
// same-named routines and variables: a call (Name(...)) and an assignment
// target (Name := ...). Delphi permits routine calls without parentheses, so
// the unknown case deliberately keeps the established routine-first behavior.
func lookupIntentAt(lines []string, position Position) lookupIntent {
	if position.Line < 0 || position.Line >= len(lines) {
		return lookupUnknown
	}
	line := lines[position.Line]
	start, end := position.Character, position.Character
	if start > len(line) {
		start = len(line)
		end = start
	}
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	for end < len(line) && isWordChar(line[end]) {
		end++
	}
	for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
		end++
	}
	if strings.HasPrefix(line[end:], ":=") {
		return lookupValue
	}
	if end < len(line) && line[end] == '(' {
		return lookupRoutine
	}
	return lookupUnknown
}

// visibleSymbolRefs returns only top-level declarations that another unit can
// legally resolve. Implementation declarations remain available within their
// own document through localDefinitionLocations above.
func (s *Server) visibleSymbolRefs(current *Document, name string) []symbolRef {
	var refs []symbolRef
	for _, ref := range s.symbolRefs(name) {
		if (current == nil || ref.uri != current.URI) && ref.symbol.Implementation {
			continue
		}
		if ref.symbol.Owner == "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func locationsForRefs(refs []symbolRef, intent lookupIntent) []Location {
	var routines, values []Location
	for _, ref := range refs {
		location := Location{URI: ref.uri, Range: ref.symbol.Selection}
		if isRoutineSymbol(ref.symbol) {
			routines = append(routines, location)
		} else {
			values = append(values, location)
		}
	}
	if intent == lookupValue && len(values) > 0 {
		return uniqueLocations(values)
	}
	if (intent == lookupRoutine || intent == lookupUnknown) && len(routines) > 0 {
		return uniqueLocations(routines)
	}
	return uniqueLocations(values)
}

func receiverAt(lines []string, position Position) string {
	receiver := receiverExpressionAt(lines, position)
	if strings.Contains(receiver, "[") {
		return ""
	}
	return receiver
}

// receiverExpressionAt returns the expression immediately before the member
// access dot. Unlike receiverAt, it includes an indexed receiver such as
// AItems[Index] so callers can resolve the array element type.
func receiverExpressionAt(lines []string, position Position) string {
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
	end := character - 1 // the dot is not part of the receiver
	start := end
	if start > 0 && line[start-1] == ']' {
		depth := 0
		for start > 0 {
			start--
			switch line[start] {
			case ']':
				depth++
			case '[':
				depth--
				if depth == 0 {
					for start > 0 && isWordChar(line[start-1]) {
						start--
					}
					return strings.TrimSpace(line[start:end])
				}
			}
		}
		return ""
	}
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	return strings.TrimSpace(line[start:end])
}

// memberTypeAt resolves the type of the expression before a dot. It covers
// ordinary variables as well as array access (for example, Items[0].Name).
func (s *Server) memberTypeAt(document *Document, position Position) string {
	expression := receiverExpressionAt(document.Lines, position)
	if expression == "" {
		return ""
	}
	routine := routineAt(document, position)
	if strings.EqualFold(expression, "self") && routine != nil {
		return routine.Owner
	}
	base, indexes := indexedExpression(expression)
	typeName := declaredTypeOf(document, base, routine)
	if typeName == "" && s.hasType(document, base) {
		typeName = base
	}
	for range indexes {
		typeName = s.arrayElementType(document, typeName)
		if typeName == "" {
			return ""
		}
	}
	return typeName
}

// indexedExpression splits a simple variable followed by zero or more index
// expressions. The actual index value is irrelevant for member resolution.
func indexedExpression(expression string) (string, int) {
	expression = strings.TrimSpace(expression)
	indexes := 0
	for strings.HasSuffix(expression, "]") {
		depth := 0
		open := -1
		for i := len(expression) - 1; i >= 0; i-- {
			switch expression[i] {
			case ']':
				depth++
			case '[':
				depth--
				if depth == 0 {
					open = i
				}
			}
			if open >= 0 {
				break
			}
		}
		if open < 0 {
			return "", 0
		}
		indexes++
		expression = strings.TrimSpace(expression[:open])
	}
	if expression == "" {
		return "", 0
	}
	return expression, indexes
}

func (s *Server) arrayElementType(current *Document, typeName string) string {
	for seen := map[string]bool{}; typeName != ""; {
		key := strings.ToLower(strings.TrimSpace(typeName))
		if seen[key] {
			return ""
		}
		seen[key] = true
		detail := typeName
		declared := s.typeDetail(current, typeName)
		if declared != "" {
			detail = declared
		}
		lower := strings.ToLower(detail)
		array := strings.Index(lower, "array")
		of := strings.Index(lower, " of ")
		if array >= 0 && of > array {
			return strings.TrimSpace(strings.TrimSuffix(detail[of+4:], ";"))
		}
		if declared == "" {
			return ""
		}
		// A type alias may point to another alias before reaching an array.
		if equal := strings.Index(detail, "="); equal >= 0 {
			typeName = strings.TrimSpace(strings.TrimSuffix(detail[equal+1:], ";"))
			continue
		}
		return ""
	}
	return ""
}

func (s *Server) typeDetail(current *Document, name string) string {
	for _, document := range s.documentsIncluding(current) {
		for _, symbol := range document.Symbols {
			if symbol.Kind == symbolClass && strings.EqualFold(symbol.Name, name) {
				return symbol.Detail
			}
		}
	}
	return ""
}

func (s *Server) hasType(current *Document, name string) bool {
	return s.typeDetail(current, name) != ""
}

func (s *Server) documentsIncluding(current *Document) []*Document {
	documents := []*Document{current}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, document := range s.docs {
		if document != current {
			documents = append(documents, document)
		}
	}
	return documents
}

func (s *Server) memberDefinitionLocations(current *Document, typeName, name string) []Location {
	var locations []Location
	seenTypes := map[string]bool{}
	var find func(string)
	find = func(typeName string) {
		key := strings.ToLower(typeName)
		if key == "" || seenTypes[key] {
			return
		}
		seenTypes[key] = true
		for _, document := range s.documentsIncluding(current) {
			for _, symbol := range document.Symbols {
				if symbol.Kind == symbolClass && strings.EqualFold(symbol.Name, typeName) {
					for _, parent := range symbol.Parents {
						find(parent)
					}
				}
				if strings.EqualFold(symbol.Owner, typeName) && strings.EqualFold(symbol.Name, name) {
					locations = append(locations, Location{URI: document.URI, Range: symbol.Selection})
				}
			}
		}
	}
	find(typeName)
	return uniqueLocations(locations)
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
	var current *Symbol
	for index := range document.Symbols {
		symbol := &document.Symbols[index]
		if symbol.Kind == symbolFunction && position.Line >= symbol.Scope.Start.Line && position.Line <= symbol.Scope.End.Line {
			// Nested routines overlap their enclosing routine's range. Prefer the
			// innermost matching range instead of depending on symbol sort order.
			if current == nil || symbol.Scope.Start.Line >= current.Scope.Start.Line {
				current = symbol
			}
		}
	}
	return current
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
