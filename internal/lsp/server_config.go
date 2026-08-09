package lsp

import (
	"fmt"
	"path/filepath"
)

func (s *Server) addConfig(name string) {
	_, dirs, err := LoadConfig(name)
	if err != nil {
		fmt.Fprintf(s.out, "") // Keep stdio clean; the client will continue with workspace folders.
		return
	}
	for _, dir := range dirs {
		s.roots["file:///"+filepath.ToSlash(dir)] = true
	}
}
