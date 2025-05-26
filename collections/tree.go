package collections

type Tree[T any] struct {
	Name     string    `json:"name,omitempty"`
	Value    T         `json:"value,omitempty"`
	Children []Tree[T] `json:"children,omitempty"`
}

func (t Tree[T]) Find(name string) *Tree[T] {
	if t.Name == name {
		return &t
	}
	for _, child := range t.Children {
		if found := child.Find(name); found != nil {
			return found
		}
	}
	return nil
}

type FlatTree[T any] struct {
	Name   string       `json:"name,omitempty"`
	Value  T            `json:"value,omitempty"`
	Parent *FlatTree[T] `json:"parent,omitempty"`
}

func (t Tree[T]) Flat() []FlatTree[T] {
	return flatTree(t, nil)
}

func flatTree[T any](node Tree[T], parent *FlatTree[T]) []FlatTree[T] {
	flat := FlatTree[T]{
		Name:   node.Name,
		Value:  node.Value,
		Parent: parent,
	}
	result := []FlatTree[T]{flat}
	for _, child := range node.Children {
		result = append(result, flatTree(child, &flat)...)
	}
	return result
}

func TreesFromFlat[T any](flat []FlatTree[T]) []Tree[T] {
	nodeMap := make(map[string]*FlatTree[T])
	for i, f := range flat {
		nodeMap[f.Name] = &flat[i]
	}
	trees := make([]Tree[T], 0)
	for _, f := range nodeMap {
		if f.Parent == nil {
			trees = append(trees, buildTree(f, nodeMap))
		}
	}
	return trees
}

func buildTree[T any](parent *FlatTree[T], nodeMap map[string]*FlatTree[T]) Tree[T] {
	root := Tree[T]{
		Name:  parent.Name,
		Value: parent.Value,
	}
	for _, child := range nodeMap {
		if child.Parent == parent {
			root.Children = append(root.Children, buildTree(child, nodeMap))
		}
	}
	return root
}

type TreeDiff[T any] struct {
	Added   []FlatTree[T]
	Removed []FlatTree[T]
	Changed []TreeDiffChange[T]
}

type TreeDiffChange[T any] struct {
	Old T
	New T
}

func DiffFlatTree[T any](old, new []FlatTree[T], isSameFunc func(old, new T) bool) TreeDiff[T] {
	diff := TreeDiff[T]{}
	oldMap := make(map[string]FlatTree[T])
	newMap := make(map[string]FlatTree[T])
	for _, item := range old {
		oldMap[item.Name] = item
	}
	for _, item := range new {
		newMap[item.Name] = item
	}
	for name, oldItem := range oldMap {
		if newItem, exists := newMap[name]; exists {
			if isSameFunc != nil && !isSameFunc(oldItem.Value, newItem.Value) {
				diff.Changed = append(diff.Changed, TreeDiffChange[T]{Old: oldItem.Value, New: newItem.Value})
			}
			delete(newMap, name)
		} else {
			diff.Removed = append(diff.Removed, oldItem)
		}
	}
	for _, newItem := range newMap {
		diff.Added = append(diff.Added, newItem)
	}
	return diff
}
