package filetree

import (
	"sort"
	"strings"
)

type Node struct {
	Name     string
	Path     string // repo-relative; "" for synthetic root
	IsDir    bool
	XY       string // git porcelain XY codes; "" = clean
	Children []*Node
}

// Symbol returns a one-char git decoration.
func (n *Node) Symbol() string {
	switch {
	case n.XY == "??":
		return "?"
	case strings.ContainsRune(n.XY, 'D'):
		return "D"
	case strings.ContainsRune(n.XY, 'U'):
		return "!"
	case strings.ContainsRune(n.XY, 'R'):
		return "R"
	case strings.HasPrefix(n.XY, "A"):
		return "A"
	case n.XY != "":
		return "M"
	default:
		return ""
	}
}

// Build creates a file tree from repo-relative paths with git status overlay.
func Build(files []string, statusMap map[string]string) *Node {
	root := &Node{IsDir: true}
	for _, f := range files {
		if f == "" {
			continue
		}
		insert(root, strings.Split(f, "/"), "", statusMap[f])
	}
	bubble(root)
	sortTree(root)
	return root
}

func insert(parent *Node, parts []string, pathPrefix string, xy string) {
	name := parts[0]
	nodePath := pathPrefix + name
	isLeaf := len(parts) == 1

	var child *Node
	for _, c := range parent.Children {
		if c.Name == name {
			child = c
			break
		}
	}
	if child == nil {
		child = &Node{Name: name, Path: nodePath, IsDir: !isLeaf}
		parent.Children = append(parent.Children, child)
	}
	if isLeaf {
		child.XY = xy
	} else {
		insert(child, parts[1:], nodePath+"/", xy)
	}
}

// bubble propagates the highest-priority git status from children up to parents.
func bubble(n *Node) {
	if !n.IsDir {
		return
	}
	for _, c := range n.Children {
		bubble(c)
		if xyRank(c.XY) > xyRank(n.XY) {
			n.XY = c.XY
		}
	}
}

func xyRank(xy string) int {
	switch xy {
	case "??":
		return 5
	case "DD", "D ", " D", "UU", "AU", "UA":
		return 4
	case "MM", " M", "AM":
		return 3
	case "M ", "A ", "R ":
		return 2
	case "":
		return 0
	default:
		return 1
	}
}

func sortTree(n *Node) {
	sort.Slice(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	for _, c := range n.Children {
		if c.IsDir {
			sortTree(c)
		}
	}
}
