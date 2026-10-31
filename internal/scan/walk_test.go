package scan

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// reference measures root with a plain sequential walk, the way the
// scanner is meant to count: every entry's blocks, files once per inode.
func reference(t testing.TB, root string) (bytes, files int64) {
	t.Helper()
	seen := map[[2]uint64]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		st := info.Sys().(*syscall.Stat_t)
		if !d.IsDir() {
			files++
			if st.Nlink > 1 {
				key := [2]uint64{st.Dev, st.Ino}
				if seen[key] {
					return nil
				}
				seen[key] = true
			}
		}
		bytes += st.Blocks * 512
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return bytes, files
}

// countEntries checks that every folder node holds exactly the entries
// on disk, with parents set, and returns how many nodes it saw.
func countEntries(t *testing.T, n *Node) int {
	t.Helper()
	if !n.IsDir {
		return 1
	}
	entries, err := os.ReadDir(n.Path())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(n.Children) {
		t.Errorf("%s: %d children, %d entries on disk", n.Path(), len(n.Children), len(entries))
	}
	total := 1
	var size, files int64
	for _, c := range n.Children {
		if c.Parent != n {
			t.Errorf("%s: wrong parent", c.Path())
		}
		size += c.Size
		files += c.Files
		total += countEntries(t, c)
	}
	if n.Files != files {
		t.Errorf("%s: files %d, children add up to %d", n.Path(), n.Files, files)
	}
	if n.Size < size {
		t.Errorf("%s: size %d below its children %d", n.Path(), n.Size, size)
	}
	return total
}

// mixedTree builds a tree that needs several getdents calls for one
// folder, long unicode names, deep nesting, hard links and symlinks.
func mixedTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	wide := filepath.Join(root, "wide")
	os.Mkdir(wide, 0o755)
	long := strings.Repeat("日本語-résumé-", 8) // well over 100 bytes
	for i := range 1500 {
		os.WriteFile(filepath.Join(wide, fmt.Sprintf("%s%04d", long, i)), []byte("x"), 0o644)
	}
	deep := root
	for i := range 40 {
		deep = filepath.Join(deep, fmt.Sprintf("d%d", i))
	}
	mkfile(t, filepath.Join(deep, "bottom.bin"), 50_000)
	mkfile(t, filepath.Join(root, "a", "data"), 120_000)
	os.Link(filepath.Join(root, "a", "data"), filepath.Join(root, "a", "data-link"))
	os.Link(filepath.Join(root, "a", "data"), filepath.Join(wide, "data-link"))
	os.Symlink("..", filepath.Join(root, "a", "up"))
	os.Symlink("/does/not/exist", filepath.Join(root, "dangling"))
	os.MkdirAll(filepath.Join(root, "empty", "nested"), 0o755)
	return root
}

func TestScanMatchesReference(t *testing.T) {
	root := mixedTree(t)
	wantBytes, wantFiles := reference(t, root)

	var s Scanner
	tree, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Size != wantBytes || tree.Files != wantFiles {
		t.Errorf("Scan = %d bytes %d files, reference %d/%d", tree.Size, tree.Files, wantBytes, wantFiles)
	}
	if got := len(find(tree, "wide").Children); got != 1501 {
		t.Errorf("wide folder has %d children, want 1501", got)
	}
	countEntries(t, tree)

	bytes, files, err := Size(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if bytes != wantBytes || files != wantFiles {
		t.Errorf("Size = %d bytes %d files, reference %d/%d", bytes, files, wantBytes, wantFiles)
	}
}

func TestSizeEdgeCases(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	file := filepath.Join(root, "f")
	want := mkfile(t, file, 9_000)

	if b, f, err := Size(ctx, file); err != nil || b != want || f != 1 {
		t.Errorf("single file: %d %d %v, want %d 1", b, f, err, want)
	}

	// A symlink to a folder is measured as the link, not followed.
	os.Mkdir(filepath.Join(root, "dir"), 0o755)
	mkfile(t, filepath.Join(root, "dir", "big"), 300_000)
	link := filepath.Join(root, "link")
	os.Symlink(filepath.Join(root, "dir"), link)
	if b, f, err := Size(ctx, link); err != nil || f != 1 || b >= 300_000 {
		t.Errorf("symlink root: %d %d %v", b, f, err)
	}

	if _, _, err := Size(ctx, filepath.Join(file, "child")); err == nil {
		t.Error("path under a file should fail")
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := Size(cancelled, root); err != context.Canceled {
		t.Errorf("cancelled: err = %v", err)
	}
}

func TestSizeUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	mkfile(t, filepath.Join(locked, "secret"), 1000)
	mkfile(t, filepath.Join(root, "open"), 1000)
	os.Chmod(locked, 0)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	// Below the root, an unreadable folder is skipped.
	if _, f, err := Size(context.Background(), root); err != nil || f != 1 {
		t.Errorf("unreadable child: files %d, err %v", f, err)
	}
	// The root itself is an error.
	if _, _, err := Size(context.Background(), locked); err == nil {
		t.Error("unreadable root should fail")
	}
}

func TestJoin(t *testing.T) {
	for _, c := range [][3]string{
		{"/", "usr", "/usr"},
		{"/home/u", "a b", "/home/u/a b"},
		{"rel", "日本", "rel/日本"},
	} {
		if got := join(c[0], c[1]); got != c[2] {
			t.Errorf("join(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

// benchTree builds 200 folders of 50 files, 10 000 entries in all.
func benchTree(b *testing.B) string {
	b.Helper()
	root := b.TempDir()
	for d := range 200 {
		dir := filepath.Join(root, fmt.Sprintf("dir%03d", d/20), fmt.Sprintf("sub%03d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		for f := range 50 {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%02d.txt", f)), []byte("data"), 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	return root
}

func BenchmarkScan(b *testing.B) {
	root := benchTree(b)
	b.ReportAllocs()
	for b.Loop() {
		var s Scanner
		if _, err := s.Scan(context.Background(), root); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSize(b *testing.B) {
	root := benchTree(b)
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Size(context.Background(), root); err != nil {
			b.Fatal(err)
		}
	}
}
