package lsp

import "strings"

func (s *Server) symbolNamed(name string) *Symbol {
	for _, document := range s.docs {
		for index := range document.Symbols {
			symbol := &document.Symbols[index]
			if strings.EqualFold(symbol.Name, name) {
				return symbol
			}
		}
	}
	return nil
}
