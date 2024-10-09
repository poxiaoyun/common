package garbagecollector

import (
	"context"
	"fmt"
	"slices"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/util/retry"
	"xiaoshiai.cn/common/controller"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/store"
)

func NewGarbagrCollector(storage store.Store, options *GraphBuilderOptions) (*GarbageCollector, error) {
	gb, err := NewGraphBuilder(storage, options)
	if err != nil {
		return nil, err
	}
	rec := &GarbageCollector{
		storage:                storage,
		dependencyGraphBuilder: gb,
		absentOwnerCache:       gb.absentOwnerCache,
	}
	return rec, nil
}

type GarbageCollector struct {
	storage                store.Store
	dependencyGraphBuilder *GraphBuilder
	absentOwnerCache       *ReferenceCache
}

func (g *GarbageCollector) Name() string {
	return "GarbageCollector"
}

func (g *GarbageCollector) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return g.dependencyGraphBuilder.Run(ctx)
	})
	eg.Go(func() error {
		return controller.RunQueueConsumer(ctx, g.dependencyGraphBuilder.attemptToDelete, g.attemptToDeleteWorker, 1)
	})
	return eg.Wait()
}

func (gc *GarbageCollector) attemptToDeleteWorker(ctx context.Context, n *node) error {
	if !n.isObserved() {
		nodeFromGraph, existsInGraph := gc.dependencyGraphBuilder.uidToNode.Get(n.identity.UID)
		if !existsInGraph {
			return nil
		}
		if nodeFromGraph.isObserved() {
			return nil
		}
	}
	if err := gc.attemptToDeleteItem(ctx, n); err != nil {
		return err
	}
	if !n.isObserved() {
		return fmt.Errorf("item %s hasn't been observed via informer yet", n.identity)
	}
	return nil
}

func (gc *GarbageCollector) attemptToDeleteItem(ctx context.Context, item *node) error {
	logger := log.FromContext(ctx)

	logger.V(2).Info("Processing item", "item", item.identity, "virtual", !item.isObserved())

	if item.isBeingDeleting() && !item.isDeletingChildrent() {
		gc.deleteObject(ctx, item.identity, "")
		return nil
	}
	latest, err := gc.getObject(ctx, item.identity)
	switch {
	case errors.IsNotFound(err):
		// the GraphBuilder can add "virtual" node for an owner that doesn't
		// exist yet, so we need to enqueue a virtual Delete event to remove
		// the virtual node from GraphBuilder.uidToNode.
		logger.V(5).Info("item not found, generating a virtual delete event",
			"item", item.identity,
		)
		gc.dependencyGraphBuilder.enqueueVirtualDeleteEvent(item.identity)
		return nil
	case err != nil:
		return err
	}

	if latest.GetUID() != item.identity.UID {
		logger.V(5).Info("UID doesn't match, item not found, generating a virtual delete event",
			"item", item.identity,
		)
		gc.dependencyGraphBuilder.enqueueVirtualDeleteEvent(item.identity)
		return nil
	}
	if item.isDeletingChildrent() {
		return gc.processDeletingDependentsItem(ctx, item)
	}
	ownerReferences := latest.GetOwnerReferences()
	if len(ownerReferences) == 0 {
		logger.V(2).Info("item doesn't have an owner, continue on next item",
			"item", item.identity,
		)
		gc.deleteObject(ctx, item.identity, "")
		return nil
	}
	solid, dangling, waitingForDependentsDeletion, err := gc.classifyReferences(ctx, item, ownerReferences)
	if err != nil {
		return err
	}
	logger.V(5).Info("classify item's references",
		"item", item.identity,
		"solid", solid,
		"dangling", dangling,
		"waitingForDependentsDeletion", waitingForDependentsDeletion,
	)
	switch {
	case len(solid) != 0:
		logger.V(2).Info("item has at least one existing owner, will not garbage collect",
			"item", item.identity,
			"owner", solid,
		)
		if len(dangling) == 0 && len(waitingForDependentsDeletion) == 0 {
			return nil
		}
		logger.V(2).Info("remove dangling references and waiting references for item",
			"item", item.identity,
			"dangling", dangling,
			"waitingForDependentsDeletion", waitingForDependentsDeletion,
		)
		// waitingForDependentsDeletion needs to be deleted from the
		// ownerReferences, otherwise the referenced objects will be stuck with
		// the FinalizerDeletingDependents and never get deleted.
		ownerUIDsToDelete := append(ownerRefsToUIDs(dangling), ownerRefsToUIDs(waitingForDependentsDeletion)...)
		_ = ownerUIDsToDelete

		// TODO: we need to patch the ownerReferences to remove any references in ownerUIDs
		return err
	case len(waitingForDependentsDeletion) != 0 && item.dependentsLength() != 0:
		deps := item.getChildren()
		for _, dep := range deps {
			if dep.isDeletingChildrent() {
				// this circle detection has false positives, we need to
				// apply a more rigorous detection if this turns out to be a
				// problem.
				// there are multiple workers run attemptToDeleteItem in
				// parallel, the circle detection can fail in a race condition.
				logger.V(2).Info("processing item, some of its owners and its dependent have FinalizerDeletingDependents, to prevent potential cycle, its ownerReferences are going to be modified to be non-blocking, then the item is going to be deleted with Foreground",
					"item", item.identity,
					"dependent", dep.identity,
				)
				// change the ownerReferences to be non-blocking
				// set all blocking ownerReferences to non-blocking
				// TODO: patch the ownerReferences to remove the blocking ownerReferences
				break
			}
		}
		logger.V(2).Info("at least one owner of item has FinalizerDeletingDependents, and the item itself has dependents, so it is going to be deleted in Foreground",
			"item", item.identity,
		)
		// the deletion event will be observed by the graphBuilder, so the item
		// will be processed again in processDeletingDependentsItem. If it
		// doesn't have dependents, the function will remove the
		// FinalizerDeletingDependents from the item, resulting in the final
		// deletion of the item.
		policy := store.DeletePropagationForeground
		return gc.deleteObject(ctx, item.identity, policy)
	default:
		// item doesn't have any solid owner, so it needs to be garbage
		// collected. Also, none of item's owners is waiting for the deletion of
		// the dependents, so set propagationPolicy based on existing finalizers.
		var policy store.DeletionPropagation
		switch {
		case hasOrphanFinalizer(latest):
			// if an existing orphan finalizer is already on the object, honor it.
			policy = store.DeletePropagationOrphan
		case hasDeleteDependentsFinalizer(latest):
			// if an existing foreground finalizer is already on the object, honor it.
			policy = store.DeletePropagationForeground
		default:
			// otherwise, default to background.
			policy = store.DeletePropagationBackground
		}
		logger.V(2).Info("Deleting item",
			"item", item.identity,
			"propagationPolicy", policy,
		)
		return gc.deleteObject(ctx, item.identity, policy)
	}
}

func hasDeleteDependentsFinalizer(accessor store.Object) bool {
	return slices.Contains(accessor.GetFinalizers(), store.FinalizerDeleteDependents)
}

func hasOrphanFinalizer(accessor store.Object) bool {
	return slices.Contains(accessor.GetFinalizers(), store.FinalizerOrphanDependents)
}

func (gc *GarbageCollector) deleteObject(ctx context.Context, item ObjectIdentity, policy store.DeletionPropagation) error {
	options := []store.DeleteOption{}
	if policy != "" {
		options = append(options, store.WithDeletePropagation(policy))
	} else {
		// directly delete the object if no policy is specified
		options = append(options, store.WithDeletePropagation(store.DeletePropagationBackground))
	}
	desc := &store.ObjectMeta{Resource: item.Resource, Name: item.Name}
	return gc.storage.Scope(item.Scopes...).Delete(ctx, desc, options...)
}

func ownerRefsToUIDs(refs []store.OwnerReference) []string {
	var ret []string
	for _, ref := range refs {
		ret = append(ret, ref.UID)
	}
	return ret
}

// classify the latestReferences to three categories:
// solid: the owner exists, and is not "waitingForDependentsDeletion"
// dangling: the owner does not exist
// waitingForDependentsDeletion: the owner exists, its deletionTimestamp is non-nil, and it has
// FinalizerDeletingDependents
// This function communicates with the server.
func (gc *GarbageCollector) classifyReferences(ctx context.Context, item *node, latestReferences []store.OwnerReference) (
	solid, dangling, waitingForDependentsDeletion []store.OwnerReference, err error,
) {
	for _, reference := range latestReferences {
		isDangling, owner, err := gc.isDangling(ctx, reference, item)
		if err != nil {
			return nil, nil, nil, err
		}
		if isDangling {
			dangling = append(dangling, reference)
			continue
		}
		if owner.GetDeletionTimestamp() != nil && slices.Contains(owner.GetFinalizers(), store.FinalizerDeleteDependents) {
			waitingForDependentsDeletion = append(waitingForDependentsDeletion, reference)
		} else {
			solid = append(solid, reference)
		}
	}
	return solid, dangling, waitingForDependentsDeletion, nil
}

// isDangling check if a reference is pointing to an object that doesn't exist.
// If isDangling looks up the referenced object at the API server, it also
// returns its latest state.
func (gc *GarbageCollector) isDangling(ctx context.Context, reference store.OwnerReference, item *node) (bool, store.Object, error) {
	logger := log.FromContext(ctx)
	// check for recorded absent cluster-scoped parent
	absentOwnerCacheKey := ownerReferenceCoordinates(reference)
	if gc.absentOwnerCache.Has(absentOwnerCacheKey) {
		logger.V(5).Info("according to the absentOwnerCache, item's owner does not exist",
			"item", item.identity,
			"owner", reference,
		)
		return true, nil, nil
	}

	// check for recorded absent namespaced parent
	absentOwnerCacheKey.Scopes = item.identity.Scopes
	if gc.absentOwnerCache.Has(absentOwnerCacheKey) {
		logger.V(5).Info("according to the absentOwnerCache, item's owner does not exist in namespace",
			"item", item.identity,
			"owner", reference,
		)
		return true, nil, nil
	}

	// TODO: It's only necessary to talk to the API server if the owner node
	// is a "virtual" node. The local graph could lag behind the real
	// status, but in practice, the difference is small.
	desc := &store.ObjectMeta{Resource: absentOwnerCacheKey.Resource}
	if err := gc.storage.Scope(absentOwnerCacheKey.Scopes...).Get(ctx, absentOwnerCacheKey.Name, desc); err != nil {
		if errors.IsNotFound(err) {
			gc.absentOwnerCache.Add(absentOwnerCacheKey)
			logger.V(5).Info("item's owner is not found",
				"item", item.identity,
				"owner", reference,
			)
			return true, nil, nil
		}
		return false, nil, err
	}

	if desc.GetUID() != reference.UID {
		logger.V(5).Info("item's owner is not found, UID mismatch",
			"item", item.identity,
			"owner", reference,
		)
		gc.absentOwnerCache.Add(absentOwnerCacheKey)
		return true, nil, nil
	}
	return false, desc, nil
}

func ownerReferenceCoordinates(ref store.OwnerReference) ObjectIdentity {
	return ObjectIdentity{UID: ref.UID, Name: ref.Name, Resource: ref.Resource}
}

func (gc *GarbageCollector) processDeletingDependentsItem(ctx context.Context, item *node) error {
	logger := log.FromContext(ctx)
	blockingDependents := item.blockingChildren()
	if len(blockingDependents) == 0 {
		logger.V(2).Info("remove DeleteDependents finalizer for item", "item", item.identity)
		return gc.removeFinalizer(ctx, item, store.FinalizerDeleteDependents)
	}
	for _, dep := range blockingDependents {
		if !dep.isDeletingChildrent() {
			logger.V(2).Info("adding dependent to attemptToDelete, because its owner is deletingDependents",
				"item", item.identity,
				"dependent", dep.identity,
			)
			gc.dependencyGraphBuilder.attemptToDelete.Add(dep)
		}
	}
	return nil
}

func (gc *GarbageCollector) getObject(ctx context.Context, item ObjectIdentity) (store.Object, error) {
	desc := &store.Unstructured{}
	desc.SetResource(item.Resource)
	if err := gc.storage.Scope(item.Scopes...).Get(ctx, item.Name, desc); err != nil {
		return nil, err
	}
	return desc, nil
}

func (gc *GarbageCollector) removeFinalizer(ctx context.Context, owner *node, targetFinalizer string) error {
	logger := log.FromContext(ctx)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		ownerObject, err := gc.getObject(ctx, owner.identity)
		if errors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot finalize owner %s, because cannot get it: %v. The garbage collector will retry later", owner.identity, err)
		}
		finalizers := ownerObject.GetFinalizers()
		newfinalizers := []string{}
		found := false
		for _, f := range finalizers {
			if f == targetFinalizer {
				found = true
				continue
			}
			newfinalizers = append(newfinalizers, f)
		}
		if !found {
			logger.V(5).Info("finalizer already removed from object", "finalizer", targetFinalizer, "object", owner.identity)

			// if len(newFinalizers) == 0 {
			// 	logger.V(5).Info("finalizers are empty, deleting object", "object", owner.identity)
			// 	// return gc.deleteObject(ctx, owner.identity, "")
			// }
			return nil
		}
		ownerObject.SetFinalizers(newfinalizers)

		return gc.storage.Scope(owner.identity.Scopes...).Status().Update(ctx, ownerObject)

		// patch, err := json.Marshal(map[string]any{"finalizers": newFinalizers})
		// if err != nil {
		// 	return fmt.Errorf("unable to finalize %s due to an error serializing patch: %v", owner.identity, err)
		// }
		// return gc.patchObject(ctx, owner.identity, patch, store.PatchTypeMergePatch)
	})
	if errors.IsConflict(err) {
		return fmt.Errorf("updateMaxRetries(%d) has reached. The garbage collector will retry later for owner %v", retry.DefaultBackoff.Steps, owner.identity)
	}
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

func (gc *GarbageCollector) patchObject(ctx context.Context, item ObjectIdentity, patch store.Patch) error {
	desc := &store.ObjectMeta{
		Resource: item.Resource,
		Name:     item.Name,
	}
	return gc.storage.Scope(item.Scopes...).Patch(ctx, desc, patch)
}
