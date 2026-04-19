package web

import (
	"embed"
	"io"
	"os"
	"path/filepath"

	"github.com/flosch/pongo2/v6"
)

//go:embed templates/pages/*.html
var templatesFS embed.FS

var templateSet *pongo2.TemplateSet

func init() {
	// Pongo2's LocalFilesystemLoader uses regular file paths, not embed.
	// Since embed.FS isn't compatible, we extract templates to a temp dir.
	dir, err := os.MkdirTemp("", "zlaw-templates-")
	if err != nil {
		panic("web: create temp dir: " + err.Error())
	}

	// Extract embedded templates to temp directory.
	if err := extractTemplates(dir); err != nil {
		panic("web: extract templates: " + err.Error())
	}

	loader, err := pongo2.NewLocalFileSystemLoader(dir)
	if err != nil {
		panic("web: init template loader: " + err.Error())
	}
	templateSet = pongo2.NewSet("web", loader)
}

// extractTemplates copies embedded templates to disk.
func extractTemplates(dir string) error {
	entries, err := templatesFS.ReadDir("templates/pages")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := templatesFS.ReadFile(filepath.Join("templates/pages", entry.Name()))
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, entry.Name()), data, 0o644); err != nil {
			continue
		}
	}
	return nil
}

// executeTemplate renders template t with data and writes to w.
func executeTemplate(w io.Writer, t string, data pongo2.Context) error {
	tpl, err := templateSet.FromCache(t)
	if err != nil {
		return err
	}
	return tpl.ExecuteWriter(data, w)
}
