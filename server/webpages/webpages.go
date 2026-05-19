// Package webpages parses templates used to render pages.
package webpages

import (
	"io"
	"io/fs"

	"github.com/mna/karbur/errors"
)

type Renderer struct {
	pages map[string]executer
}

type executer interface {
	Execute(io.Writer, any) error
}

// New creates a new Renderer with templates from the provided filesystem. The
// common/ sub-directory must contain reusable templates (e.g. layouts,
// partials), while the pages/ sub-directory contains the entrypoints (actual
// pages to render). Template names are the path relative to the initial
// sub-directory, e.g. "common/layouts/base.tpl" would be compiled to a
// template named "layouts/base.tpl", while "pages/app/login.tpl" would be
// named "app/login.tpl".
func New(tpls fs.FS) (*Renderer, error) {
	panic("unimplemented")
}

func (r *Renderer) Render(w io.Writer, page string, data any) error {
	p, ok := r.pages[page]
	if !ok {
		return errors.Errorf("webpages: no such page: %q", page)
	}
	return p.Execute(w, data)
}
