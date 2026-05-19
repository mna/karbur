// Package webpages parses templates used to render pages.
package webpages

import (
	"html/template"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/mna/karbur/errors"
)

type Renderer struct {
	pages map[string]executer
}

type executer interface {
	Execute(io.Writer, any) error
}

// New creates a new Renderer with templates from the provided filesystem. Only
// files with the .tpl extension are compiled. The common/ sub-directory must
// contain reusable templates (e.g. layouts, partials), while the pages/
// sub-directory contains the entrypoints (actual pages to render). Template
// names are the path relative to the initial sub-directory, e.g.
// "common/layouts/base.tpl" would be compiled to a template named
// "layouts/base.tpl", while "pages/app/login.tpl" would be named
// "app/login.tpl".
func New(tpls fs.FS) (*Renderer, error) {
	readFile := readFileFS(tpls)

	var commonT *template.Template
	errCommon := walkSubDir(tpls, "common", readFile, func(name, content string) error {
		var t *template.Template
		if commonT == nil {
			commonT = template.New(name)
			t = commonT
		} else {
			t = commonT.New(name)
		}
		_, err := t.Parse(content)
		return err
	})
	if errCommon != nil {
		return nil, errCommon
	}

	pages := make(map[string]executer)
	errPages := walkSubDir(tpls, "pages", readFile, func(name, content string) error {
		pageT, err := commonT.Clone()
		if err != nil {
			return errors.Errorf("webpages: clone common templates: %w", err)
		}
		t := pageT.New(name)
		pages[name] = t
		_, err = t.Parse(content)
		return err
	})
	if errPages != nil {
		return nil, errPages
	}

	return &Renderer{pages: pages}, nil
}

func walkSubDir(tpls fs.FS, subDir string,
	readFile func(string, string) (string, []byte, error),
	parseTemplate func(string, string) error,
) error {
	return fs.WalkDir(tpls, subDir, func(p string, d fs.DirEntry, err error) error {
		if p == subDir && errors.Is(err, fs.ErrNotExist) {
			// no such directory, not an error
			return nil
		}
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || path.Ext(d.Name()) != ".tpl" {
			return nil
		}

		name, b, err := readFile(p, subDir+"/")
		if err != nil {
			return err
		}
		s := string(b)
		return parseTemplate(name, s)
	})
}

func readFileFS(fsys fs.FS) func(string, string) (string, []byte, error) {
	return func(file, strip string) (name string, b []byte, err error) {
		name = strings.TrimPrefix(file, strip)
		b, err = fs.ReadFile(fsys, file)
		return
	}
}

func (r *Renderer) Render(w io.Writer, page string, data any) error {
	p, ok := r.pages[page]
	if !ok {
		return errors.Errorf("webpages: no such page: %q", page)
	}
	return p.Execute(w, data)
}
