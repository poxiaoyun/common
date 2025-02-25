package rest

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

type Server struct {
	Store store.Store
}

func NewServer(store store.Store) *Server {
	return &Server{Store: store}
}

type CountResponse struct {
	Count int `json:"count"`
}

func (s *Server) List(w http.ResponseWriter, r *http.Request) {
	s.on(w, r, func(ctx context.Context, ref store.ResourcedObjectReference) (any, error) {
		log := log.FromContext(ctx)
		if ref.Name == "" {
			options := store.ListOptions{
				Page:             api.Query(r, "page", 0),
				Size:             api.Query(r, "size", 0),
				Search:           api.Query(r, "search", ""),
				Sort:             api.Query(r, "sort", ""),
				IncludeSubScopes: api.Query(r, "includeSubScopes", false),
				ResourceVersion:  api.Query(r, "resourceVersion", int64(0)),
			}
			labelsel, fildsel, err := decodeSelector(r)
			if err != nil {
				return nil, err
			}
			options.LabelRequirements = labelsel
			options.FieldRequirements = fildsel

			list := store.List[store.Unstructured]{}
			list.Resource = ref.Resource

			// count
			if count := api.Query(r, "count", false); count {
				obj := &store.Unstructured{}
				obj.SetResource(ref.Resource)
				count, err := s.Store.Scope(ref.Scopes...).Count(ctx, obj, func(co *store.CountOptions) {
					*co = store.CountOptions{
						LabelRequirements: options.LabelRequirements,
						FieldRequirements: options.FieldRequirements,
						IncludeSubScopes:  options.IncludeSubScopes,
					}
				})
				if err != nil {
					return nil, err
				}
				return CountResponse{Count: count}, nil
			}
			// watch
			if watch := api.Query(r, "watch", false); watch {
				watcher, err := s.Store.Scope(ref.Scopes...).Watch(ctx, &list, func(wo *store.WatchOptions) {
					*wo = store.WatchOptions{
						LabelRequirements: options.LabelRequirements,
						FieldRequirements: options.FieldRequirements,
						IncludeSubScopes:  options.IncludeSubScopes,
						ResourceVersion:   options.ResourceVersion,
						SendInitialEvents: api.Query(r, "sendInitialEvents", false),
					}
				})
				if err != nil {
					return nil, err
				}
				defer watcher.Stop()

				ssew := api.NewSSEWriter(w)
				for {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case event, ok := <-watcher.Events():
						if !ok {
							ssew.WriteEvent("error", fmt.Errorf("watcher closed"))
							return nil, nil
						}
						if event.Error != nil {
							ssew.WriteEvent("error", event.Error)
							return nil, nil
						}
						if err := ssew.WriteEvent(string(event.Type), event.Object); err != nil {
							log.Error(err, "write event")
							return nil, nil
						}
					}
				}
			}
			// list
			if err := s.Store.Scope(ref.Scopes...).List(ctx, &list, func(lo *store.ListOptions) {
				*lo = options
			}); err != nil {
				return nil, err
			}
			return list, nil

		} else {
			// get
			obj := &store.Unstructured{}
			obj.SetResource(ref.Resource)
			if err := s.Store.Scope(ref.Scopes...).Get(ctx, ref.Name, obj); err != nil {
				return nil, err
			}
			return obj, nil

		}
	})
}

func (s *Server) Create(w http.ResponseWriter, r *http.Request) {
	s.on(w, r, func(ctx context.Context, ref store.ResourcedObjectReference) (any, error) {
		obj := &store.Unstructured{}
		if err := api.Body(r, obj); err != nil {
			return nil, err
		}
		obj.SetResource(ref.Resource)

		options := store.CreateOptions{
			TTL:                 api.Query(r, "ttl", time.Duration(0)),
			AutoIncrementOnName: api.Query(r, "autoIncrementOnName", false),
		}
		if err := s.Store.Scope(ref.Scopes...).Create(ctx, obj, func(co *store.CreateOptions) {
			*co = options
		}); err != nil {
			return nil, err
		}
		return obj, nil
	})
}

const PatchDataLimit = 5 * 1024 * 1024 // 5MB

func (s *Server) Patch(w http.ResponseWriter, r *http.Request) {
	s.on(w, r, func(ctx context.Context, ref store.ResourcedObjectReference) (any, error) {
		if ref.Name == "" {
			return nil, errors.NewBadRequest("name is required")
		}
		patchtype, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			return nil, fmt.Errorf("invalid content type: %s", err)
		}
		patchdata, err := io.ReadAll(io.LimitReader(r.Body, PatchDataLimit))
		if err != nil {
			return nil, err
		}
		patch := store.RawPatch(store.PatchType(patchtype), patchdata)

		options := store.PatchOptions{}
		labelsel, fildsel, err := decodeSelector(r)
		if err != nil {
			return nil, err
		}
		options.LabelRequirements = labelsel
		options.FieldRequirements = fildsel

		obj := &store.Unstructured{}
		obj.SetResource(ref.Resource)
		obj.SetName(ref.Name)

		if status := api.Query(r, "status", false); status {
			if err := s.Store.Scope(ref.Scopes...).Status().Patch(ctx, obj, patch, func(po *store.PatchOptions) {
				*po = options
			}); err != nil {
				return nil, err
			}
		} else {
			if err := s.Store.Scope(ref.Scopes...).Patch(ctx, obj, patch, func(po *store.PatchOptions) {
				*po = options
			}); err != nil {
				return nil, err
			}
		}
		return obj, nil
	})
}

func (s *Server) Update(w http.ResponseWriter, r *http.Request) {
	s.on(w, r, func(ctx context.Context, ref store.ResourcedObjectReference) (any, error) {
		if ref.Name == "" {
			return nil, errors.NewBadRequest("name is required")
		}
		options := store.UpdateOptions{
			TTL: api.Query(r, "ttl", time.Duration(0)),
		}
		labelsel, fildsel, err := decodeSelector(r)
		if err != nil {
			return nil, err
		}
		options.LabelRequirements = labelsel
		options.FieldRequirements = fildsel
		obj := &store.Unstructured{}
		if err := api.Body(r, obj); err != nil {
			return nil, err
		}
		if obj.GetName() != ref.Name {
			return nil, errors.NewBadRequest(fmt.Sprintf("name in body %s is not equal to name in path %s", obj.GetName(), ref.Name))
		}
		obj.SetResource(ref.Resource)

		if status := api.Query(r, "status", false); status {
			if err := s.Store.Scope(ref.Scopes...).Status().Update(ctx, obj, func(uo *store.UpdateOptions) {
				*uo = options
			}); err != nil {
				return nil, err
			}
		} else {
			if err := s.Store.Scope(ref.Scopes...).Update(ctx, obj, func(uo *store.UpdateOptions) {
				*uo = options
			}); err != nil {
				return nil, err
			}
		}
		return obj, nil
	})
}

func (s *Server) Delete(w http.ResponseWriter, r *http.Request) {
	s.on(w, r, func(ctx context.Context, ref store.ResourcedObjectReference) (any, error) {
		if ref.Name == "" {
			return nil, errors.NewBadRequest("name is required")
		}
		options := store.DeleteOptions{}
		if propagationPolicy := api.Query(r, "propagationPolicy", ""); propagationPolicy != "" {
			policy := store.DeletionPropagation(propagationPolicy)
			options.PropagationPolicy = &policy
		}
		obj := &store.Unstructured{}
		obj.SetResource(ref.Resource)
		obj.SetName(ref.Name)
		if err := s.Store.Scope(ref.Scopes...).Delete(ctx, obj, func(do *store.DeleteOptions) {
			*do = options
		}); err != nil {
			return nil, err
		}
		return obj, nil
	})
}

func (s *Server) on(w http.ResponseWriter, r *http.Request,
	fn func(ctx context.Context, ref store.ResourcedObjectReference) (any, error),
) {
	api.On(w, r, func(ctx context.Context) (any, error) {
		return fn(ctx, decodePath(api.Path(r, "path", "")))
	})
}

func decodeSelector(r *http.Request) (store.Requirements, store.Requirements, error) {
	var labelRequirements store.Requirements
	if labelsel := api.Query(r, "labelSelector", ""); labelsel != "" {
		sel, err := labels.Parse(labelsel)
		if err != nil {
			return nil, nil, errors.NewBadRequest(err.Error())
		}
		labelRequirements = store.LabelsSelectorToReqirements(sel)
	}
	var fieldRequirements store.Requirements
	if fieldsel := api.Query(r, "fieldSelector", ""); fieldsel != "" {
		fields, err := fields.ParseSelector(fieldsel)
		if err != nil {
			return nil, nil, errors.NewBadRequest(err.Error())
		}
		fieldRequirements = store.FieldsSelectorToReqirements(fields)
	}
	return labelRequirements, fieldRequirements, nil
}

// decodePath
// /scope/name/scope/name/resource/name
// /scope/name/scope/name/resource
// /scope/name/resource
func decodePath(rpath string) store.ResourcedObjectReference {
	rpath = strings.TrimPrefix(rpath, "/")
	rpath = strings.TrimSuffix(rpath, "/")
	parts := strings.Split(rpath, "/")

	scopes := []store.Scope{}
	// every two parts is a scope and name
	for i := 0; i < len(parts); i += 2 {
		if i+1 == len(parts) {
			scopes = append(scopes, store.Scope{Resource: parts[i]})
		} else {
			scopes = append(scopes, store.Scope{Resource: parts[i], Name: parts[i+1]})
		}
	}
	scopes, last := scopes[:len(scopes)-1], scopes[len(scopes)-1]
	return store.ResourcedObjectReference{
		Scopes:   scopes,
		Resource: last.Resource,
		Name:     last.Name,
	}
}

func (s *Server) Group() api.Group {
	return api.NewGroup("/{path}*").
		Route(
			api.GET("").To(s.List),
			api.POST("").To(s.Create),
			api.PUT("").To(s.Update),
			api.DELETE("").To(s.Delete),
			api.PATCH("").To(s.Patch),
		)
}
