package webpages

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed testdata
var testdata embed.FS

func TestRenderer(t *testing.T) {
	funcs := template.FuncMap{
		"customfn": func(i int) string { return fmt.Sprint(i) },
	}

	t.Run("empty", func(t *testing.T) {
		tpls, _ := fs.Sub(testdata, "testdata/empty")
		r, err := New(tpls, nil)
		require.NoError(t, err)
		require.NotNil(t, r)

		var buf bytes.Buffer
		err = r.Render(&buf, "page.tpl", nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "no such page")
	})

	t.Run("commononly", func(t *testing.T) {
		tpls, _ := fs.Sub(testdata, "testdata/commononly")
		r, err := New(tpls, funcs)
		require.NoError(t, err)
		require.NotNil(t, r)

		var buf bytes.Buffer
		err = r.Render(&buf, "page.tpl", nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "no such page")
	})

	t.Run("pagesonly", func(t *testing.T) {
		tpls, _ := fs.Sub(testdata, "testdata/pagesonly")
		r, err := New(tpls, funcs)
		require.NoError(t, err)
		require.NotNil(t, r)

		var buf bytes.Buffer
		err = r.Render(&buf, "page.tpl", nil)
		require.NoError(t, err)
		require.Equal(t, "Page 2\n", buf.String())

		buf.Reset()
		err = r.Render(&buf, "sub/other.tpl", nil)
		require.NoError(t, err)
		require.Equal(t, "Other 3\n", buf.String())
	})

	t.Run("both", func(t *testing.T) {
		tpls, _ := fs.Sub(testdata, "testdata/both")
		r, err := New(tpls, funcs)
		require.NoError(t, err)
		require.NotNil(t, r)

		var buf bytes.Buffer
		err = r.Render(&buf, "page.tpl", nil)
		require.NoError(t, err)
		require.Equal(t, "Layout 1\n\nMessages\n\nPage 2\n", buf.String())

		buf.Reset()
		err = r.Render(&buf, "sub/other.tpl", nil)
		require.NoError(t, err)
		require.Equal(t, "Layout 1\n\nMessages\n\nOther 3\n", buf.String())
	})
}
