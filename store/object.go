// +k8s:openapi-gen=true
package store

import "xiaoshiai.cn/common/meta"

// Object exposes the metadata managed by Store implementations.
type Object interface {
	GetID() string
	SetID(string)

	GetName() string
	SetName(string)

	GetUID() string
	SetUID(string)

	GetDescription() string
	SetDescription(string)

	GetResource() string
	SetResource(string)

	GetScopes() []Scope
	SetScopes([]Scope)

	GetResourceVersion() int64
	SetResourceVersion(int64)

	GetGeneration() int64
	SetGeneration(int64)

	GetLabels() map[string]string
	SetLabels(map[string]string)

	GetAnnotations() map[string]string
	SetAnnotations(map[string]string)

	GetDeletionTimestamp() *meta.Time
	SetDeletionTimestamp(*meta.Time)

	GetCreationTimestamp() meta.Time
	SetCreationTimestamp(meta.Time)

	GetFinalizers() []string
	SetFinalizers([]string)

	GetOwnerReferences() []OwnerReference
	SetOwnerReferences([]OwnerReference)
}

// ObjectList exposes items and flat pagination metadata managed by a Store
// List operation.
type ObjectList interface {
	GetResource() string
	SetResource(string)

	GetScopes() []Scope
	SetScopes([]Scope)

	GetResourceVersion() int64
	SetResourceVersion(int64)

	// GetPage returns the one-based page number, or zero outside page pagination.
	GetPage() int
	// SetPage sets the one-based page number.
	SetPage(page int)
	// GetSize returns the page size, or zero outside page pagination.
	GetSize() int
	// SetSize sets the page size.
	SetSize(size int)
	// GetTotal returns the exact total, or nil for continuation pagination.
	GetTotal() *int
	// SetTotal sets or omits the exact total.
	SetTotal(total *int)
	// GetContinue returns the opaque next-batch token, or empty when a
	// continuation traversal is complete.
	GetContinue() string
	// SetContinue sets the opaque next-batch token.
	SetContinue(token string)
	// GetLimit returns the continuation batch limit, or zero outside
	// continuation pagination.
	GetLimit() int
	// SetLimit sets the continuation batch limit.
	SetLimit(limit int)
}

// +k8s:openapi-gen=true
type Scope struct {
	Resource string `json:"resource,omitempty"`
	Name     string `json:"name,omitempty"`
}

// +k8s:openapi-gen=true
type OwnerReference struct {
	ID                 string  `json:"id,omitempty"`
	Resource           string  `json:"resource,omitempty"`
	UID                string  `json:"uid,omitempty"`
	Scopes             []Scope `json:"scopes,omitempty"`
	Controller         bool    `json:"controller,omitempty"`
	BlockOwnerDeletion *bool   `json:"blockOwnerDeletion,omitempty"`
}

var _ Object = &ObjectMeta{}

// +k8s:openapi-gen=true
type ObjectMeta struct {
	ID                string            `json:"id,omitempty"  validation:"name"`
	Name              string            `json:"name,omitempty"`
	UID               string            `json:"uid,omitempty"`
	APIVersion        string            `json:"apiVersion,omitempty"`
	Scopes            []Scope           `json:"scopes,omitempty"`
	Resource          string            `json:"resource,omitempty"`
	ResourceVersion   int64             `json:"resourceVersion,omitempty"`
	Generation        int64             `json:"generation,omitempty"`
	CreationTimestamp meta.Time         `json:"creationTimestamp,omitempty"`
	DeletionTimestamp *meta.Time        `json:"deletionTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Finalizers        []string          `json:"finalizers,omitempty"`
	OwnerReferences   []OwnerReference  `json:"ownerReferences,omitempty"`
	Description       string            `json:"description,omitempty"`
}

func (o *ObjectMeta) GetID() string {
	return o.ID
}

func (o *ObjectMeta) SetID(id string) {
	o.ID = id
}

// GetVersion implements Object.
func (o *ObjectMeta) GetAPIVersion() string {
	return o.APIVersion
}

// SetVersion implements Object.
func (o *ObjectMeta) SetAPIVersion(version string) {
	o.APIVersion = version
}

// GetDescription implements Object.
func (o *ObjectMeta) GetDescription() string {
	return o.Description
}

// SetDescription implements Object.
func (o *ObjectMeta) SetDescription(desc string) {
	o.Description = desc
}

// GetAnnotations implements Object.
func (o *ObjectMeta) GetAnnotations() map[string]string {
	return o.Annotations
}

// GetCreationTimestamp implements Object.
func (o *ObjectMeta) GetCreationTimestamp() meta.Time {
	return o.CreationTimestamp
}

// GetDeletionTimestamp implements Object.
func (o *ObjectMeta) GetDeletionTimestamp() *meta.Time {
	return o.DeletionTimestamp
}

// GetFinalizers implements Object.
func (o *ObjectMeta) GetFinalizers() []string {
	return o.Finalizers
}

// GetLabels implements Object.
func (o *ObjectMeta) GetLabels() map[string]string {
	return o.Labels
}

// GetName implements Object.
func (o *ObjectMeta) GetName() string {
	return o.Name
}

// GetOwnerReferences implements Object.
func (o *ObjectMeta) GetOwnerReferences() []OwnerReference {
	return o.OwnerReferences
}

// GetResource implements Object.
func (o *ObjectMeta) GetResource() string {
	return o.Resource
}

// GetResourceVersion implements Object.
func (o *ObjectMeta) GetResourceVersion() int64 {
	return o.ResourceVersion
}

// GetScopes implements Object.
func (o *ObjectMeta) GetScopes() []Scope {
	return o.Scopes
}

// GetUID implements Object.
func (o *ObjectMeta) GetUID() string {
	return o.UID
}

// SetAnnotations implements Object.
func (o *ObjectMeta) SetAnnotations(anotations map[string]string) {
	o.Annotations = anotations
}

// SetCreationTimestamp implements Object.
func (o *ObjectMeta) SetCreationTimestamp(creationTimestamp meta.Time) {
	o.CreationTimestamp = creationTimestamp
}

// SetDeletionTimestamp implements Object.
func (o *ObjectMeta) SetDeletionTimestamp(deletionTimestamp *meta.Time) {
	o.DeletionTimestamp = deletionTimestamp
}

// SetFinalizers implements Object.
func (o *ObjectMeta) SetFinalizers(finalizers []string) {
	o.Finalizers = finalizers
}

// SetLabels implements Object.
func (o *ObjectMeta) SetLabels(labels map[string]string) {
	o.Labels = labels
}

// SetName implements Object.
func (o *ObjectMeta) SetName(name string) {
	o.Name = name
}

// SetOwnerReferences implements Object.
func (o *ObjectMeta) SetOwnerReferences(ownerReferences []OwnerReference) {
	o.OwnerReferences = ownerReferences
}

// SetResource implements Object.
func (o *ObjectMeta) SetResource(resource string) {
	o.Resource = resource
}

// SetResourceVersion implements Object.
func (o *ObjectMeta) SetResourceVersion(resourceVersion int64) {
	o.ResourceVersion = resourceVersion
}

// GetGeneration implements Object.
func (o *ObjectMeta) GetGeneration() int64 {
	return o.Generation
}

// SetGeneration implements Object.
func (o *ObjectMeta) SetGeneration(generation int64) {
	o.Generation = generation
}

// SetScopes implements Object.
func (o *ObjectMeta) SetScopes(scopes []Scope) {
	o.Scopes = scopes
}

// SetUID implements Object.
func (o *ObjectMeta) SetUID(uid string) {
	o.UID = uid
}

var _ ObjectList = &List[Object]{}

// List is the Store wire-compatible list result with flat pagination metadata.
type List[T any] struct {
	Resource        string  `json:"resource,omitempty"`
	ResourceVersion int64   `json:"resourceVersion,omitempty"`
	Scopes          []Scope `json:"scopes,omitempty"`
	Items           []T     `json:"items" openapi:"dynamic"`
	Total           *int    `json:"total,omitempty"`
	Page            int     `json:"page,omitempty"`
	Size            int     `json:"size,omitempty"`
	Continue        string  `json:"continue,omitempty"` // Used for pagination, if set, indicates that there are more items to list
	Limit           int     `json:"limit,omitempty"`
}

// PageFromList projects Store list metadata onto the public list contract.
func PageFromList[T any](list List[T]) meta.Page[T] {
	return meta.Page[T]{
		ResourceVersion: list.ResourceVersion,
		Total:           list.Total,
		Items:           list.Items,
		Page:            list.Page,
		Size:            list.Size,
		Continue:        list.Continue,
		Limit:           list.Limit,
	}
}

// GetContinue implements ObjectList.
func (b *List[T]) GetContinue() string {
	return b.Continue
}

// SetContinue implements ObjectList.
func (b *List[T]) SetContinue(continueToken string) {
	b.Continue = continueToken
}

// GetLimit implements ObjectList.
func (b *List[T]) GetLimit() int {
	return b.Limit
}

// SetLimit implements ObjectList.
func (b *List[T]) SetLimit(limit int) {
	b.Limit = limit
}

// GetResourceVersion implements ObjectList.
func (b *List[T]) GetResourceVersion() int64 {
	return b.ResourceVersion
}

// SetResourceVersion implements ObjectList.
func (b *List[T]) SetResourceVersion(resourceVersion int64) {
	b.ResourceVersion = resourceVersion
}

// GetScopes implements ObjectList.
func (b *List[T]) GetScopes() []Scope {
	return b.Scopes
}

// SetScopes implements ObjectList.
func (b *List[T]) SetScopes(scopes []Scope) {
	b.Scopes = scopes
}

// GetResource implements ObjectList.
func (b *List[T]) GetResource() string {
	return b.Resource
}

// SetResource implements ObjectList.
func (b *List[T]) SetResource(resource string) {
	b.Resource = resource
}

// GetPage implements ObjectList.
func (b *List[T]) GetPage() int {
	return b.Page
}

// GetSize implements ObjectList.
func (b *List[T]) GetSize() int {
	return b.Size
}

// GetTotal implements ObjectList.
func (b *List[T]) GetTotal() *int {
	return b.Total
}

// SetPage implements ObjectList.
func (b *List[T]) SetPage(i int) {
	b.Page = i
}

// SetSize implements ObjectList.
func (b *List[T]) SetSize(size int) {
	b.Size = size
}

// SetTotal implements ObjectList.
func (b *List[T]) SetTotal(total *int) {
	b.Total = total
}

// SetUnpaginatedListMetadata records an unpaginated result with an exact total.
func SetUnpaginatedListMetadata(list ObjectList, total int) {
	list.SetPage(0)
	list.SetSize(0)
	list.SetContinue("")
	list.SetLimit(0)
	list.SetTotal(&total)
}

// SetPageListMetadata records a page result with an exact total.
func SetPageListMetadata(list ObjectList, page, size, total int) {
	list.SetPage(page)
	list.SetSize(size)
	list.SetContinue("")
	list.SetLimit(0)
	list.SetTotal(&total)
}

// SetContinuationListMetadata records a continuation result and omits totals.
func SetContinuationListMetadata(list ObjectList, token string, limit int) {
	list.SetPage(0)
	list.SetSize(0)
	list.SetTotal(nil)
	list.SetContinue(token)
	list.SetLimit(limit)
}

// ConvertList maps list items while preserving Store and pagination metadata.
func ConvertList[T any, F any](list List[T], f func(T) F) List[F] {
	newItems := make([]F, 0, len(list.Items))
	for _, item := range list.Items {
		newItems = append(newItems, f(item))
	}
	return List[F]{
		Resource:        list.Resource,
		ResourceVersion: list.ResourceVersion,
		Scopes:          list.Scopes,
		Page:            list.Page,
		Size:            list.Size,
		Total:           list.Total,
		Items:           newItems,
		Continue:        list.Continue,
		Limit:           list.Limit,
	}
}
