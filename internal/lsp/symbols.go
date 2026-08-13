package lsp

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
func (s *Server) symbolAtLocation(current *Document, location Location) *Symbol {
	document := current
	if location.URI != current.URI {
		document = s.document(location.URI)
	}
	if document != nil {
		for index := range document.Symbols {
			symbol := &document.Symbols[index]
			if symbol.Selection == location.Range {
				return symbol
			}
		}
	}
	return nil
}
