package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeStaticBundle creates relPath under root, with an index.html when withIndex is set.
func writeStaticBundle(t *testing.T, root, relPath string, withIndex bool) {
	t.Helper()
	dir := filepath.Join(root, relPath)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	if withIndex {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<div id=\"root\"></div>"), 0o644))
	}
}

func TestResolveStaticDir(t *testing.T) {
	const rel = notificationCenterRelPath

	t.Run("a bundle in the working directory is the relative path itself", func(t *testing.T) {
		root := t.TempDir()
		writeStaticBundle(t, root, rel, true)
		t.Chdir(root)

		assert.Equal(t, rel, resolveStaticDir(rel))
	})

	t.Run("a bundle two levels up is found, as from tests/integration", func(t *testing.T) {
		root := t.TempDir()
		writeStaticBundle(t, root, rel, true)
		sub := filepath.Join(root, "tests", "integration")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		t.Chdir(sub)

		got := resolveStaticDir(rel)
		assert.Equal(t, filepath.Join("..", "..", rel), got)
		_, err := os.Stat(filepath.Join(got, "index.html"))
		assert.NoError(t, err)
	})

	t.Run("a directory without index.html does not count", func(t *testing.T) {
		root := t.TempDir()
		writeStaticBundle(t, root, rel, true)
		sub := filepath.Join(root, "tests", "integration")
		// An empty notification_center/dist next to the working directory, as an
		// interrupted build leaves
		writeStaticBundle(t, sub, rel, false)
		t.Chdir(sub)

		assert.Equal(t, filepath.Join("..", "..", rel), resolveStaticDir(rel))
	})

	t.Run("no bundle anywhere falls back to the relative path", func(t *testing.T) {
		t.Chdir(t.TempDir())

		assert.Equal(t, rel, resolveStaticDir(rel))
	})

	t.Run("a bundle beyond the search depth is not found", func(t *testing.T) {
		root := t.TempDir()
		writeStaticBundle(t, root, rel, true)
		deep := root
		for i := 0; i <= webAnalyticsSDKSearchDepth; i++ {
			deep = filepath.Join(deep, "d")
		}
		require.NoError(t, os.MkdirAll(deep, 0o755))
		t.Chdir(deep)

		assert.Equal(t, rel, resolveStaticDir(rel))
	})
}
