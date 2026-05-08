package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

type Options struct {
	Address   string
	Directory string
}

func Run(options Options) error {
	wwwPath, err := filepath.Abs(options.Directory)
	if err != nil {
		return fmt.Errorf("resolve directory: %w", err)
	}

	info, err := os.Stat(wwwPath)
	if err != nil {
		return fmt.Errorf("stat directory %q: %w", wwwPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", wwwPath)
	}

	fmt.Printf("Serving %s on http://%s\n", wwwPath, options.Address)
	return http.ListenAndServe(options.Address, http.FileServer(http.Dir(wwwPath)))
}
