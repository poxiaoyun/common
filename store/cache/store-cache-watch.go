package cache

import (
	"context"
	"fmt"
	"sync"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/store"
)

// Watch implements Store. CacheStore intentionally exposes no Watch history;
// callers rebuild with an initial Watch after ResourceExpired.
func (g *CacheStore) Watch(ctx context.Context, list store.ObjectList, opts ...store.WatchOption) (store.Watcher, error) {
	resource, err := store.GetResource(list)
	if err != nil {
		return nil, err
	}
	options := store.ApplyWatchOptions(opts)
	if options.ResourceVersion != nil && *options.ResourceVersion > 0 {
		return nil, errors.NewResourceExpired(resource, "watch history is unavailable")
	}
	_, newItem, err := store.NewItemFuncFromList(list)
	if err != nil {
		return nil, err
	}
	return g.core.resource(resource).watch(ctx, g.scopes, newItem, options)
}

func (c *cachedResource) watch(
	ctx context.Context,
	scopes []store.Scope,
	newItem func() store.Object,
	options store.WatchOptions,
) (store.Watcher, error) {
	for {
		if err := c.waitUntilReady(ctx); err != nil {
			return nil, err
		}

		watchCtx, cancel := context.WithCancel(ctx)
		watcher := &cachedWatcher{
			parent:   c,
			cancel:   cancel,
			scopes:   append([]store.Scope(nil), scopes...),
			options:  options,
			newItem:  newItem,
			input:    make(chan store.WatchEvent, 100),
			terminal: make(chan error, 1),
			results:  make(chan store.WatchEvent, 100),
		}

		c.stateLock.Lock()
		if !c.isReady {
			c.stateLock.Unlock()
			cancel()
			continue
		}
		initial := []store.WatchEvent{}
		if options.SendInitialEvents {
			for _, object := range c.items {
				matches, err := watcher.matches(object)
				if err != nil {
					c.stateLock.Unlock()
					cancel()
					return nil, err
				}
				if !matches {
					continue
				}
				converted, err := watcher.convert(object)
				if err != nil {
					c.stateLock.Unlock()
					cancel()
					return nil, err
				}
				initial = append(initial, store.WatchEvent{Type: store.WatchEventCreate, Object: converted})
			}
		}
		c.nextWatcherID++
		watcher.id = c.nextWatcherID
		c.watchers[watcher.id] = watcher
		c.stateLock.Unlock()

		go watcher.run(watchCtx, initial, options.SendInitialEvents)
		return watcher, nil
	}
}

func (c *cachedResource) detachWatcher(watcher *cachedWatcher) {
	c.stateLock.Lock()
	delete(c.watchers, watcher.id)
	c.stateLock.Unlock()
}

var _ store.Watcher = &cachedWatcher{}

type cachedWatcher struct {
	id       int64
	parent   *cachedResource
	cancel   context.CancelFunc
	stopOnce sync.Once

	scopes  []store.Scope
	options store.WatchOptions
	newItem func() store.Object

	input    chan store.WatchEvent
	terminal chan error
	results  chan store.WatchEvent
}

func (w *cachedWatcher) run(ctx context.Context, initial []store.WatchEvent, bookmark bool) {
	defer close(w.results)
	defer w.parent.detachWatcher(w)
	for _, event := range initial {
		if !w.send(ctx, event) {
			return
		}
	}
	if bookmark && !w.send(ctx, store.WatchEvent{Type: store.WatchEventBookmark}) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-w.terminal:
			w.send(ctx, store.WatchEvent{Error: err})
			return
		case event := <-w.input:
			if !w.send(ctx, event) {
				return
			}
		}
	}
}

func (w *cachedWatcher) send(ctx context.Context, event store.WatchEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case err := <-w.terminal:
		select {
		case w.results <- store.WatchEvent{Error: err}:
		case <-ctx.Done():
		}
		return false
	case w.results <- event:
		return true
	}
}

func (w *cachedWatcher) enqueueTransition(old, current *store.Unstructured) bool {
	oldMatches, err := w.matches(old)
	if err != nil {
		w.expire(err)
		return false
	}
	currentMatches, err := w.matches(current)
	if err != nil {
		w.expire(err)
		return false
	}
	var event store.WatchEvent
	switch {
	case !oldMatches && currentMatches:
		object, err := w.convert(current)
		if err != nil {
			w.expire(err)
			return false
		}
		event = store.WatchEvent{Type: store.WatchEventCreate, Object: object}
	case oldMatches && currentMatches:
		object, err := w.convert(current)
		if err != nil {
			w.expire(err)
			return false
		}
		event = store.WatchEvent{Type: store.WatchEventUpdate, Object: object}
	case oldMatches && !currentMatches:
		object, err := w.convert(old)
		if err != nil {
			w.expire(err)
			return false
		}
		event = store.WatchEvent{Type: store.WatchEventDelete, Object: object}
	default:
		return true
	}
	select {
	case w.input <- event:
		return true
	default:
		return false
	}
}

func (w *cachedWatcher) matches(object *store.Unstructured) (bool, error) {
	if object == nil || w.options.ID != "" && object.GetID() != w.options.ID {
		return false, nil
	}
	if w.options.IncludeSubScopes {
		if !store.ScopesIsSameOrUnder(object.GetScopes(), w.scopes) {
			return false, nil
		}
	} else if !store.ScopesEquals(object.GetScopes(), w.scopes) {
		return false, nil
	}
	return store.MatchLabelReqirements(object, w.options.LabelRequirements) &&
		store.MatchUnstructuredFieldRequirments(object, w.options.FieldRequirements), nil
}

func (w *cachedWatcher) convert(object *store.Unstructured) (store.Object, error) {
	result := w.newItem()
	if err := store.FromUnstructured(object, result); err != nil {
		return nil, errors.NewInternalError(fmt.Errorf("convert cached watch object: %w", err))
	}
	return result, nil
}

func (w *cachedWatcher) expire(err error) {
	select {
	case w.terminal <- err:
	default:
	}
}

// Stop implements Watcher.
func (w *cachedWatcher) Stop() {
	w.stopOnce.Do(w.cancel)
}

// Events implements Watcher.
func (w *cachedWatcher) Events() <-chan store.WatchEvent {
	return w.results
}
