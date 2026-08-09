package main

import (
	"flag"
	"log"
	"os"

	"github.com/example/delphi-lsp/internal/lsp"
)

func main() {
	config := flag.String("config", "", "path to delphi-lsp.json")
	flag.Parse()
	if *config != "" {
		_ = os.Setenv("DELPHI_LSP_CONFIG", *config)
	}
	log.SetOutput(os.Stderr)
	if err := lsp.NewServer(os.Stdin, os.Stdout).Serve(); err != nil {
		log.Printf("delphi-lsp: %v", err)
		os.Exit(1)
	}
}
