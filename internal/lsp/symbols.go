package lsp

func (s *Server) symbolNamed(name string) *Symbol {
	refs := s.symbolRefs(name)
	if len(refs) == 0 {
		return nil
	}
	return &refs[0].symbol
}
