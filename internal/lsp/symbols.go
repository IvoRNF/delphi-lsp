package lsp

import "strings"

func (s *Server) symbolNamed(name string) *Symbol {
	refs := s.symbolRefs(name)
	if len(refs) == 0 {
		return nil
	}
	for _, ref := range refs {
		if !ref.symbol.Implementation {
			return &ref.symbol
		}
	}
	return nil
}

// symbolAtLocation converts a resolved navigation target into the matching
// symbol metadata. Hover must use this instead of a name-only index lookup:
// a variable and a routine may legitimately share a spelling in different
// visible scopes.
//
// The optional name disambiguates declarations that share a selection range.
// The synthetic Result variable created for a function reuses the function's
// name range, and symbols are sorted by name, so without it a hover over a
// routine could return "Result: <type>" instead of the routine signature.
func (s *Server) symbolAtLocation(current *Document, location Location, name ...string) *Symbol {
	document := current
	if location.URI != current.URI {
		document = s.document(location.URI)
	}
	if document != nil {
		var fallback *Symbol
		for index := range document.Symbols {
			symbol := &document.Symbols[index]
			if symbol.Selection != location.Range {
				continue
			}
			if len(name) == 0 || strings.EqualFold(symbol.Name, name[0]) {
				return symbol
			}
			if fallback == nil {
				fallback = symbol
			}
		}
		return fallback
	}
	return nil
}
