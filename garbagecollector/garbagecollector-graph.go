package garbagecollector

import (
	"fmt"
	"sync"

	"github.com/golang/groupcache/lru"
	"xiaoshiai.cn/common/store"
)

type ScopedObjectReference struct {
	ObjectIdentity  `json:",inline"`
	OwnerReferences []store.OwnerReference `json:"ownerReferences"`
	Finalizers      []string               `json:"finalizers"`
	Deleting        bool                   `json:"deleting"`
	Virtual         bool                   `json:"virtual"`
	Deleted         bool                   `json:"deleted"`
}

type ObjectIdentity struct {
	Resource string        `json:"resource"`
	Name     string        `json:"name"`
	UID      string        `json:"uid"`
	Scopes   []store.Scope `json:"scopes"`
}

func (n ObjectIdentity) Equals(other ObjectIdentity) bool {
	return n.Resource == other.Resource && n.Name == other.Name && n.UID == other.UID && IsSameScopes(n.Scopes, other.Scopes)
}

func (n ObjectIdentity) Identity() string {
	id := n.UID + ":" + n.Resource
	for _, scope := range n.Scopes {
		id += "/" + scope.Resource + "/" + scope.Name
	}
	id += "/" + n.Name
	return id
}

type empty struct{}

type node struct {
	identity ObjectIdentity
	// children are the nodes that have an ownerReference to this node
	children     map[*node]empty
	childrenLock sync.RWMutex
	// deletingChildren is true if the node is being deleted and its children are being deleted
	deletingChildren     bool
	deletingChildrenLock sync.RWMutex
	// this records if the object's deletionTimestamp is non-nil.
	beingDeleting     bool
	beingDeletingLock sync.RWMutex
	// this records if the object was constructed virtually and never observed via informer event
	virtual     bool
	virtualLock sync.RWMutex
	// owners are the ownerReferences of this node
	owners []store.OwnerReference
}

// clone() must only be called from the single-threaded GraphBuilder.processGraphChanges()
func (n *node) clone() *node {
	c := &node{
		identity:         n.identity,
		children:         make(map[*node]empty, len(n.children)),
		deletingChildren: n.deletingChildren,
		beingDeleting:    n.beingDeleting,
		virtual:          n.virtual,
		owners:           make([]store.OwnerReference, 0, len(n.owners)),
	}
	for dep := range n.children {
		c.children[dep] = struct{}{}
	}
	for _, owner := range n.owners {
		c.owners = append(c.owners, owner)
	}
	return c
}

func (n *node) markBeingDeleting() {
	n.beingDeletingLock.Lock()
	defer n.beingDeletingLock.Unlock()
	n.beingDeleting = true
}

func (n *node) isBeingDeleting() bool {
	n.beingDeletingLock.RLock()
	defer n.beingDeletingLock.RUnlock()
	return n.beingDeleting
}

func (n *node) markObserved() {
	n.virtualLock.Lock()
	defer n.virtualLock.Unlock()
	n.virtual = false
}

func (n *node) isObserved() bool {
	n.virtualLock.RLock()
	defer n.virtualLock.RUnlock()
	return !n.virtual
}

func (n *node) markDeletingChildren() {
	n.deletingChildrenLock.Lock()
	defer n.deletingChildrenLock.Unlock()
	n.deletingChildren = true
}

func (n *node) isDeletingChildrent() bool {
	n.deletingChildrenLock.RLock()
	defer n.deletingChildrenLock.RUnlock()
	return n.deletingChildren
}

func (n *node) addChild(child *node) {
	n.childrenLock.Lock()
	defer n.childrenLock.Unlock()
	n.children[child] = struct{}{}
}

func (n *node) deleteChild(child *node) {
	n.childrenLock.Lock()
	defer n.childrenLock.Unlock()
	delete(n.children, child)
}

func (n *node) dependentsLength() int {
	n.childrenLock.RLock()
	defer n.childrenLock.RUnlock()
	return len(n.children)
}

func (n *node) getChildren() []*node {
	n.childrenLock.RLock()
	defer n.childrenLock.RUnlock()
	var ret []*node
	for dep := range n.children {
		ret = append(ret, dep)
	}
	return ret
}

// blockingChildren returns the children that block deletion of this node.
func (n *node) blockingChildren() []*node {
	dependents := n.getChildren()
	var ret []*node
	for _, dep := range dependents {
		for _, owner := range dep.owners {
			if owner.UID == n.identity.UID && owner.BlockOwnerDeletion != nil && *owner.BlockOwnerDeletion {
				ret = append(ret, dep)
			}
		}
	}
	return ret
}

// String renders node as a string using fmt. Acquires a read lock to ensure the
// reflective dump of dependents doesn't race with any concurrent writes.
func (n *node) String() string {
	n.childrenLock.RLock()
	defer n.childrenLock.RUnlock()
	return fmt.Sprintf("%#v", n)
}

type concurrentUIDToNode struct {
	uidToNodeLock sync.RWMutex
	uidToNode     map[string]*node
}

func (m *concurrentUIDToNode) Set(node *node) {
	m.uidToNodeLock.Lock()
	defer m.uidToNodeLock.Unlock()
	m.uidToNode[node.identity.UID] = node
}

func (m *concurrentUIDToNode) Get(uid string) (*node, bool) {
	m.uidToNodeLock.RLock()
	defer m.uidToNodeLock.RUnlock()
	n, ok := m.uidToNode[uid]
	return n, ok
}

func (m *concurrentUIDToNode) Delete(uid string) {
	m.uidToNodeLock.Lock()
	defer m.uidToNodeLock.Unlock()
	delete(m.uidToNode, uid)
}

// ReferenceCache is an LRU cache for uid.
type ReferenceCache struct {
	mutex sync.Mutex
	cache *lru.Cache
}

// NewReferenceCache returns a ReferenceCache.
func NewReferenceCache(maxCacheEntries int) *ReferenceCache {
	return &ReferenceCache{
		cache: lru.New(maxCacheEntries),
	}
}

// Add adds a uid to the cache.
func (c *ReferenceCache) Add(reference ObjectIdentity) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.cache.Add(reference.Identity(), nil)
}

// Has returns if a uid is in the cache.
func (c *ReferenceCache) Has(reference ObjectIdentity) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	_, found := c.cache.Get(reference.Identity())
	return found
}
