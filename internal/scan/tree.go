package scan

import (
	"container/heap"
	"sort"
	"strings"
)

// SortBy chooses the order of a folder listing.
type SortBy int

const (
	BySize SortBy = iota
	ByName
	ByFiles
)

// Sorted returns the children ordered by the given key. Ties are broken by
// name so the order stays stable between redraws.
func (n *Node) Sorted(by SortBy) []*Node {
	out := make([]*Node, len(n.Children))
	copy(out, n.Children)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch by {
		case BySize:
			if a.Size != b.Size {
				return a.Size > b.Size
			}
		case ByFiles:
			if a.Files != b.Files {
				return a.Files > b.Files
			}
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out
}

// Remove detaches a child and subtracts its size from every ancestor.
func (n *Node) Remove(child *Node) {
	for i, c := range n.Children {
		if c != child {
			continue
		}
		n.Children = append(n.Children[:i], n.Children[i+1:]...)
		for p := n; p != nil; p = p.Parent {
			p.Size -= child.Size
			p.Files -= child.Files
		}
		child.Parent = nil
		return
	}
}

// LargestFiles returns up to limit files under n, biggest first.
func (n *Node) LargestFiles(limit int) []*Node {
	if limit <= 0 {
		return nil
	}
	h := &minHeap{}
	var visit func(*Node)
	visit = func(x *Node) {
		if !x.IsDir {
			if h.Len() < limit {
				heap.Push(h, x)
			} else if x.Size > (*h)[0].Size {
				(*h)[0] = x
				heap.Fix(h, 0)
			}
			return
		}
		for _, c := range x.Children {
			visit(c)
		}
	}
	visit(n)
	out := make([]*Node, h.Len())
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = heap.Pop(h).(*Node)
	}
	return out
}

type minHeap []*Node

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].Size < h[j].Size }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(*Node)) }
func (h *minHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
