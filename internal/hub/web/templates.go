package web

import (
	"embed"
	"io"

	"github.com/flosch/pongo2/v6"
)

//go:embed templates/pages/*.html
var templatesFS embed.FS

var templateSet = pongo2.NewSet("web", pongo2.NewFSLoader(templatesFS))

// executeTemplate renders template t with data and writes to w.
func executeTemplate(w io.Writer, t string, data pongo2.Context) error {
	tpl, err := templateSet.FromCache(t)
	if err != nil {
		return err
	}
	return tpl.ExecuteWriter(data, w)
}
