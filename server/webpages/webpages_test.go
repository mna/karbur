package webpages

import (
	"bytes"
	"embed"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed testdata
var testdata embed.FS

func TestRenderer(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		tpls, _ := fs.Sub(testdata, "testdata/empty")
		r, err := New(tpls)
		require.NoError(t, err)
		require.NotNil(t, r)

		var buf bytes.Buffer
		err = r.Render(&buf, "page.tpl", nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "no such page")
	})

	t.Run("commononly", func(t *testing.T) {
		tpls, _ := fs.Sub(testdata, "testdata/commononly")
		r, err := New(tpls)
		require.NoError(t, err)
		require.NotNil(t, r)

		var buf bytes.Buffer
		err = r.Render(&buf, "page.tpl", nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "no such page")
	})

	t.Run("pagesonly", func(t *testing.T) {
		tpls, _ := fs.Sub(testdata, "testdata/pagesonly")
		r, err := New(tpls)
		require.NoError(t, err)
		require.NotNil(t, r)

		var buf bytes.Buffer
		err = r.Render(&buf, "page.tpl", nil)
		require.NoError(t, err)
		require.Equal(t, "Page\n", buf.String())

		buf.Reset()
		err = r.Render(&buf, "sub/other.tpl", nil)
		require.NoError(t, err)
		require.Equal(t, "Other\n", buf.String())
	})

	t.Run("both", func(t *testing.T) {
		tpls, _ := fs.Sub(testdata, "testdata/both")
		r, err := New(tpls)
		require.NoError(t, err)
		require.NotNil(t, r)

		var buf bytes.Buffer
		err = r.Render(&buf, "page.tpl", nil)
		require.NoError(t, err)
		require.Equal(t, "Layout\n\nMessages\n\nPage\n", buf.String())

		buf.Reset()
		err = r.Render(&buf, "sub/other.tpl", nil)
		require.NoError(t, err)
		require.Equal(t, "Layout\n\nMessages\n\nOther\n", buf.String())
	})
}
