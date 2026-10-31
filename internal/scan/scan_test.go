package scan

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// mkfile writes a file of n bytes and returns its allocated size.
func mkfile(t *testing.T, path string, n int) int64 {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
	return allocated(t, path)
}

func allocated(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Sys().(*syscall.Stat_t).Blocks * 512
}

func find(n *Node, name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestScanTree(t *testing.T) {
	root := t.TempDir()
	big := mkfile(t, filepath.Join(root, "videos", "big.bin"), 300_000)
	small := mkfile(t, filepath.Join(root, "docs", "note.txt"), 5_000)
	uni := mkfile(t, filepath.Join(root, "docs", "Résumé 日本.pdf"), 9_000)
	os.Mkdir(filepath.Join(root, "empty"), 0o755)
	// A symlink loop must not be followed.
	os.Symlink(".", filepath.Join(root, "docs", "loop"))

	var s Scanner
	tree, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	videos, docs, empty := find(tree, "videos"), find(tree, "docs"), find(tree, "empty")
	if videos == nil || docs == nil || empty == nil {
		t.Fatalf("missing children: %+v", tree.Children)
	}
	if find(videos, "big.bin").Size != big {
		t.Errorf("big.bin size = %d, want %d", find(videos, "big.bin").Size, big)
	}
	if docs.Files != 3 { // note, résumé, symlink
		t.Errorf("docs files = %d, want 3", docs.Files)
	}
	if docs.Size < small+uni {
		t.Errorf("docs size %d smaller than its files %d", docs.Size, small+uni)
	}
	if empty.Files != 0 || len(empty.Children) != 0 {
		t.Errorf("empty dir has content: %+v", empty)
	}
	if got := find(docs, "Résumé 日本.pdf").Path(); got != filepath.Join(root, "docs", "Résumé 日本.pdf") {
		t.Errorf("Path() = %q", got)
	}

	// Scanner and Size must agree on the total.
	bytes, files, err := Size(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if bytes != tree.Size || files != tree.Files {
		t.Errorf("Size() = %d/%d, Scan = %d/%d", bytes, files, tree.Size, tree.Files)
	}
	pf, pb, _ := s.Progress()
	if pf != tree.Files || pb > tree.Size {
		t.Errorf("progress %d files %d bytes, tree %d/%d", pf, pb, tree.Files, tree.Size)
	}
}

func TestHardlinksCountedOnce(t *testing.T) {
	root := t.TempDir()
	one := mkfile(t, filepath.Join(root, "a", "data"), 200_000)
	if err := os.Link(filepath.Join(root, "a", "data"), filepath.Join(root, "b-data")); err != nil {
		t.Skip("hard links not supported:", err)
	}
	var s Scanner
	tree, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	var fileBytes int64
	var visit func(*Node)
	visit = func(n *Node) {
		if !n.IsDir {
			fileBytes += n.Size
		}
		for _, c := range n.Children {
			visit(c)
		}
	}
	visit(tree)
	if fileBytes != one {
		t.Errorf("file bytes = %d, want %d (counted once)", fileBytes, one)
	}
	bytes, _, _ := Size(context.Background(), root)
	if bytes != tree.Size {
		t.Errorf("Size() = %d, Scan = %d", bytes, tree.Size)
	}
}

func TestUnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	mkfile(t, filepath.Join(locked, "secret"), 1000)
	os.Chmod(locked, 0)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	var s Scanner
	tree, err := s.Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if n := find(tree, "locked"); n == nil || n.Err == nil {
		t.Errorf("locked folder should carry an error: %+v", n)
	}
	if _, _, errs := s.Progress(); errs != 1 {
		t.Errorf("errors = %d, want 1", errs)
	}
}

func TestScanErrors(t *testing.T) {
	var s Scanner
	if _, err := s.Scan(context.Background(), "/does/not/exist"); err == nil {
		t.Error("expected error for missing root")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "x"), 10)
	if _, err := s.Scan(ctx, root); err != context.Canceled {
		t.Errorf("cancelled scan err = %v", err)
	}

	if b, f, err := Size(context.Background(), "/does/not/exist"); b != 0 || f != 0 || err != nil {
		t.Errorf("missing path: %d %d %v", b, f, err)
	}
}

func TestScanSingleFile(t *testing.T) {
	root := t.TempDir()
	want := mkfile(t, filepath.Join(root, "f"), 4096)
	var s Scanner
	n, err := s.Scan(context.Background(), filepath.Join(root, "f"))
	if err != nil || n.IsDir || n.Size != want || n.Files != 1 {
		t.Errorf("got %+v, %v", n, err)
	}
}
