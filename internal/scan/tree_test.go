package scan

import "testing"

func sample() *Node {
	root := &Node{Name: "/r", IsDir: true}
	add := func(p *Node, name string, size, files int64, dir bool) *Node {
		c := &Node{Name: name, Size: size, Files: files, IsDir: dir, Parent: p}
		p.Children = append(p.Children, c)
		return c
	}
	a := add(root, "a", 0, 0, true)
	add(a, "a1", 50, 1, false)
	add(a, "a2", 10, 1, false)
	a.Size, a.Files = 60, 2
	add(root, "B", 100, 1, false)
	add(root, "c", 60, 1, false)
	root.Size, root.Files = 220, 4
	return root
}

func names(ns []*Node) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Name)
	}
	return out
}

func TestSorted(t *testing.T) {
	r := sample()
	if got := names(r.Sorted(BySize)); got[0] != "B" || got[1] != "a" || got[2] != "c" {
		t.Errorf("by size: %v (ties by name)", got)
	}
	if got := names(r.Sorted(ByName)); got[0] != "a" || got[1] != "B" {
		t.Errorf("by name is case-insensitive: %v", got)
	}
	if got := names(r.Sorted(ByFiles)); got[0] != "a" {
		t.Errorf("by files: %v", got)
	}
	if r.Children[0].Name != "a" {
		t.Error("Sorted must not reorder the original slice")
	}
}

func TestRemove(t *testing.T) {
	r := sample()
	a := r.Children[0]
	a1 := a.Children[0]
	a.Remove(a1)
	if a.Size != 10 || a.Files != 1 || r.Size != 170 || r.Files != 3 {
		t.Errorf("after remove: a=%d/%d root=%d/%d", a.Size, a.Files, r.Size, r.Files)
	}
	if a1.Parent != nil || len(a.Children) != 1 {
		t.Error("child not detached")
	}
	a.Remove(a1) // removing twice is a no-op
	if r.Size != 170 {
		t.Error("second remove changed sizes")
	}
}

func TestLargestFiles(t *testing.T) {
	r := sample()
	if got := names(r.LargestFiles(2)); len(got) != 2 || got[0] != "B" || (got[1] != "c" && got[1] != "a1") {
		t.Errorf("top 2: %v", got)
	}
	if got := r.LargestFiles(10); len(got) != 4 || got[3].Name != "a2" {
		t.Errorf("all files: %v", names(got))
	}
	if r.LargestFiles(0) != nil {
		t.Error("limit 0 should return nil")
	}
}
