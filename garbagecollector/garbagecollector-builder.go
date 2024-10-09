package garbagecollector

import (
	"context"
	"reflect"
	"strings"

	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"xiaoshiai.cn/common/controller"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

type GraphBuilderOptions struct {
	IgnoredResources []string
	WatchResources   []string
}

func NewGraphBuilder(storage store.Store, options *GraphBuilderOptions) (*GraphBuilder, error) {
	gb := &GraphBuilder{
		options:          options,
		storage:          storage,
		uidToNode:        &concurrentUIDToNode{uidToNode: make(map[string]*node)},
		graphChanges:     controller.NewDefaultTypedQueue[*ScopedObjectReference]("graph-changes", nil),
		attemptToDelete:  controller.NewDefaultTypedQueue[*node]("attempt-to-delete", nil),
		absentOwnerCache: NewReferenceCache(500),
	}
	return gb, nil
}

type GraphBuilder struct {
	options          *GraphBuilderOptions
	storage          store.Store
	uidToNode        *concurrentUIDToNode
	graphChanges     controller.TypedQueue[*ScopedObjectReference]
	attemptToDelete  controller.TypedQueue[*node]
	absentOwnerCache *ReferenceCache
}

func (g *GraphBuilder) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return g.MonitorResources(ctx)
	})
	eg.Go(func() error {
		return g.StartProcess(ctx)
	})
	return eg.Wait()
}

func (g *GraphBuilder) MonitorResources(ctx context.Context) error {
	log := log.FromContext(ctx)
	resourcesmap := make(map[string]empty)
	for _, resource := range g.options.WatchResources {
		resourcesmap[resource] = empty{}
	}
	eg, ctx := errgroup.WithContext(ctx)
	for resource := range resourcesmap {
		resource := resource
		if slices.Contains(g.options.IgnoredResources, resource) {
			log.Info("Ignoring resource", "resource", resource)
			continue
		}
		eg.Go(func() error {
			return g.startMonitor(ctx, resource)
		})
	}
	return eg.Wait()
}

func (g *GraphBuilder) startMonitor(ctx context.Context, resource string) error {
	logger := log.FromContext(ctx).WithValues("resource", resource)
	logger.Info("start monitor")
	ctx = log.NewContext(ctx, logger)

	return controller.RunListWatchContext(ctx, g.storage, resource, controller.EventHandlerFunc[*store.Unstructured](func(ctx context.Context, kind store.WatchEventType, obj *store.Unstructured) error {
		if obj.GetUID() == "" {
			return nil
		}
		ref := &ScopedObjectReference{
			ObjectIdentity: ObjectIdentity{
				Resource: obj.GetResource(),
				Name:     obj.GetName(),
				UID:      obj.GetUID(),
				Scopes:   obj.GetScopes(),
			},
			Deleting:        obj.GetDeletionTimestamp() != nil,
			OwnerReferences: obj.GetOwnerReferences(),
			Finalizers:      obj.GetFinalizers(),
			Deleted:         kind == store.WatchEventDelete,
		}
		g.graphChanges.Add(ref)
		return nil
	}))
}

func (gb *GraphBuilder) StartProcess(ctx context.Context) error {
	return controller.RunQueueConsumer(ctx, gb.graphChanges, gb.ProcessGraphChanges, 1)
}

func (gb *GraphBuilder) ProcessGraphChanges(ctx context.Context, key *ScopedObjectReference) error {
	log := log.FromContext(ctx)
	log.V(5).Info("processing graph change", "key", key.Name, "resource", key.Resource)
	existingNode, ok := gb.uidToNode.Get(key.UID)

	// If the node exists in the graph and is not observed, mark it as observed.
	if ok && !key.Virtual && !existingNode.isObserved() {
		// If the node's identity has changed, update it.
		if !key.ObjectIdentity.Equals(existingNode.identity) {
			_, invalidDependents := partitionDependents(existingNode.getChildren(), key.ObjectIdentity)
			for _, dep := range invalidDependents {
				gb.attemptToDelete.Add(dep)
			}
			existingNode.identity = key.ObjectIdentity
		}
		existingNode.markObserved()
	}

	switch {
	case !ok && !key.Deleted:
		node := &node{
			identity: key.ObjectIdentity,
			children: map[*node]empty{},
			owners:   key.OwnerReferences,
		}
		gb.uidToNode.Set(node)
		gb.addDependentToOwners(node, node.owners)
		gb.processTransitions(key, node)
	case ok && !key.Deleted:
		added, removed, changed := referencesDiffs(existingNode.owners, key.OwnerReferences)
		if len(added) != 0 || len(removed) != 0 || len(changed) != 0 {
			existingNode.owners = key.OwnerReferences
			gb.addUnblockedOwnersToDeleteQueue(removed, changed)
			gb.addDependentToOwners(existingNode, added)
			gb.removeDependentFromOwners(existingNode, removed)
		}
		gb.processTransitions(key, existingNode)
		return nil
	case key.Deleted:
		if !ok {
			return nil
		}
		removeExistingNode := true
		if key.Virtual {
			// if the node is virtual, we need to check if it has any dependents that are not being deleted
			if existingNode.virtual {
				matchingDependents, nonmatchingDependents := partitionDependents(existingNode.getChildren(), key.ObjectIdentity)
				if len(nonmatchingDependents) > 0 {
					// some dependents are not agree to delete, so we should not remove the node
					removeExistingNode = false
					for _, dep := range matchingDependents {
						gb.attemptToDelete.Add(dep)
					}
				}
			} else {
				if !existingNode.identity.Equals(key.ObjectIdentity) {
					removeExistingNode = false
					matchingDependents, _ := partitionDependents(existingNode.getChildren(), key.ObjectIdentity)
					for _, dep := range matchingDependents {
						gb.attemptToDelete.Add(dep)
					}
				}
			}
		}

		if removeExistingNode {
			gb.uidToNode.Delete(existingNode.identity.UID)
			gb.removeDependentFromOwners(existingNode, existingNode.owners)
			existingNode.childrenLock.RLock()
			defer existingNode.childrenLock.RUnlock()
			for dep := range existingNode.children {
				gb.attemptToDelete.Add(dep)
			}
			for _, owner := range existingNode.owners {
				ownerNode, found := gb.uidToNode.Get(owner.UID)
				if !found || !ownerNode.isDeletingChildrent() {
					continue
				}
				// notify the owner that one of its children is being deleted
				gb.attemptToDelete.Add(ownerNode)
			}
		}
	}

	return nil
}

func (gb *GraphBuilder) processTransitions(key *ScopedObjectReference, n *node) {
	if key.Deleting {
		n.markBeingDeleting()
		if slices.Contains(key.Finalizers, store.FinalizerDeleteDependents) {
			n.markDeletingChildren()
			for dep := range n.children {
				gb.attemptToDelete.Add(dep)
			}
			gb.attemptToDelete.Add(n)
		} else {
			if len(key.Finalizers) == 0 {
				gb.attemptToDelete.Add(n)
			}
		}
	}
}

func (gb *GraphBuilder) enqueueVirtualDeleteEvent(ref ObjectIdentity) {
	gb.graphChanges.Add(&ScopedObjectReference{
		ObjectIdentity: ref,
		Virtual:        true,
		Deleted:        true,
	})
}

func (gb *GraphBuilder) addUnblockedOwnersToDeleteQueue(removed []store.OwnerReference, changed []ownerRefPair) {
	for _, ref := range removed {
		if ref.BlockOwnerDeletion != nil && *ref.BlockOwnerDeletion {
			node, found := gb.uidToNode.Get(ref.UID)
			if !found {
				continue
			}
			gb.attemptToDelete.Add(node)
		}
	}
	for _, c := range changed {
		wasBlocked := c.oldRef.BlockOwnerDeletion != nil && *c.oldRef.BlockOwnerDeletion
		isUnblocked := c.newRef.BlockOwnerDeletion == nil || (c.newRef.BlockOwnerDeletion != nil && !*c.newRef.BlockOwnerDeletion)
		if wasBlocked && isUnblocked {
			node, found := gb.uidToNode.Get(c.newRef.UID)
			if !found {
				continue
			}
			gb.attemptToDelete.Add(node)
		}
	}
}

func partitionDependents(dependents []*node, matchOwnerIdentity ObjectIdentity) (matching, nonmatching []*node) {
	for i := range dependents {
		dep := dependents[i]
		foundMatch := false
		foundMismatch := false
		if !IsSameScopes(dep.identity.Scopes, matchOwnerIdentity.Scopes) {
			foundMismatch = true
		} else {
			for _, ownerRef := range dep.owners {
				if ownerRef.UID != matchOwnerIdentity.UID {
					continue
				}
				if ownerReferenceMatchesCoordinates(ownerRef, matchOwnerIdentity) {
					foundMatch = true
				} else {
					foundMismatch = true
				}
			}
		}

		if foundMatch {
			matching = append(matching, dep)
		}
		if foundMismatch {
			nonmatching = append(nonmatching, dep)
		}
	}
	return matching, nonmatching
}

type ownerRefPair struct {
	oldRef store.OwnerReference
	newRef store.OwnerReference
}

func referencesDiffs(old []store.OwnerReference, news []store.OwnerReference) (added []store.OwnerReference, removed []store.OwnerReference, changed []ownerRefPair) {
	oldrefs := make(map[string]store.OwnerReference)
	for _, value := range old {
		oldrefs[string(value.UID)] = value
	}
	for _, new := range news {
		if old, ok := oldrefs[new.UID]; ok {
			if !reflect.DeepEqual(old, new) {
				changed = append(changed, ownerRefPair{oldRef: old, newRef: new})
			} else {
				delete(oldrefs, new.UID)
			}
		} else {
			added = append(added, new)
		}
	}
	for _, old := range oldrefs {
		removed = append(removed, old)
	}
	return added, removed, changed
}

func (gb *GraphBuilder) removeNode(n *node) {
	gb.uidToNode.Delete(n.identity.UID)
	gb.removeDependentFromOwners(n, n.owners)
}

// removeDependentFromOwners remove n from owners' dependents list.
func (gb *GraphBuilder) removeDependentFromOwners(n *node, owners []store.OwnerReference) {
	for _, owner := range owners {
		ownerNode, ok := gb.uidToNode.Get(owner.UID)
		if !ok {
			continue
		}
		ownerNode.deleteChild(n)
	}
}

// addDependentToOwners adds n to owners' dependents list.
func (gb *GraphBuilder) addDependentToOwners(n *node, owners []store.OwnerReference) {
	hasInvalidOwnerReference := false
	for _, owner := range owners {
		ownerNode, ok := gb.uidToNode.Get(owner.UID)
		// owner is not exist, need verify the object
		if !ok {
			// Create a "virtual" node in the graph for the owner if it doesn't exist in the graph yet.
			ownerNode = &node{
				identity: ObjectIdentity{Resource: owner.Resource, Name: owner.Name, UID: owner.UID, Scopes: owner.Scopes},
				children: make(map[*node]empty),
				virtual:  true,
			}
			gb.uidToNode.Set(ownerNode)
			// verify if the owner is really exist
			gb.attemptToDelete.Add(ownerNode)
		}
		ownerNode.addChild(n)
		if !hasInvalidOwnerReference {
			if !IsUnderScoped(n.identity.Scopes, ownerNode.identity.Scopes) ||
				!ownerReferenceMatchesCoordinates(owner, ownerNode.identity) {
				hasInvalidOwnerReference = true
			}
		}
	}
	if hasInvalidOwnerReference {
		// some referenced invalid,need verify the object
		gb.attemptToDelete.Add(n)
	}
}

// ownerReferenceMatchesCoordinates returns true if all of the coordinate fields match
// between the two references (uid, name, kind, apiVersion)
func ownerReferenceMatchesCoordinates(a store.OwnerReference, b ObjectIdentity) bool {
	return a.UID == b.UID && a.Name == b.Name && a.Resource == b.Resource
}

func SortScopes(scopes []store.Scope) []store.Scope {
	slices.SortFunc(scopes, func(i, j store.Scope) int {
		return strings.Compare(i.Resource, j.Resource)
	})
	return scopes
}

func IsSameScopes(scope1, scope2 []store.Scope) bool {
	if len(scope1) == 0 && len(scope2) == 0 {
		return true
	}
	if len(scope1) != len(scope2) {
		return false
	}
	return reflect.DeepEqual(SortScopes(scope1), SortScopes(scope2))
}

// IsUnderScoped returns true if scope1 is under scope2.
// eg. scope1 = [ { "namespace", "default" } ], scope2 = [ { "namespace", "default" }, { "cluster", "abc" } ].
// scope2 is under scope1.
func IsUnderScoped(scope1, scope2 []store.Scope) bool {
	if len(scope2) == 0 {
		return true
	}
	if len(scope1) < len(scope2) {
		return false
	}
	scope1, scope2 = SortScopes(scope1), SortScopes(scope2)
	for i := range scope2 {
		if scope1[i] != scope2[i] {
			return false
		}
	}
	return true
}
