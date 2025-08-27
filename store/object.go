// +k8s:openapi-gen=true
package store

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	GetLabels() map[string]string
	SetLabels(map[string]string)
	GetAnnotations() map[string]string
	SetAnnotations(map[string]string)
	GetDeletionTimestamp() *Time
	SetDeletionTimestamp(*Time)
	GetCreationTimestamp() Time
	SetCreationTimestamp(Time)
	GetFinalizers() []string
	SetFinalizers([]string)
	GetOwnerReferences() []OwnerReference
	SetOwnerReferences([]OwnerReference)
}

type ObjectList interface {
	GetResource() string
	SetResource(string)

	GetScopes() []Scope
	SetScopes([]Scope)

	GetResourceVersion() int64
	SetResourceVersion(int64)

	GetSize() int
	SetSize(size int)
	SetPage(i int)
	GetPage() int
	SetTotal(i int)
	GetTotal() int

	SetContinue(string)
	GetContinue() string
}

// +k8s:openapi-gen=true
type Time = metav1.Time

type Duration = metav1.Duration

func Now() Time {
	return Time(metav1.Now())
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
	ID                string            `json:"id,omitempty"`
	Name              string            `json:"name,omitempty" validate:"name"`
	UID               string            `json:"uid,omitempty"`
	APIVersion        string            `json:"apiVersion,omitempty"`
	Scopes            []Scope           `json:"scopes,omitempty"`
	Resource          string            `json:"resource,omitempty"`
	ResourceVersion   int64             `json:"resourceVersion,omitempty"`
	CreationTimestamp Time              `json:"creationTimestamp,omitempty"`
	DeletionTimestamp *Time             `json:"deletionTimestamp,omitempty"`
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
func (o *ObjectMeta) GetCreationTimestamp() Time {
	return o.CreationTimestamp
}

// GetDeletionTimestamp implements Object.
func (o *ObjectMeta) GetDeletionTimestamp() *Time {
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
func (o *ObjectMeta) SetCreationTimestamp(creationTimestamp Time) {
	o.CreationTimestamp = creationTimestamp
}

// SetDeletionTimestamp implements Object.
func (o *ObjectMeta) SetDeletionTimestamp(deletionTimestamp *Time) {
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

// SetScopes implements Object.
func (o *ObjectMeta) SetScopes(scopes []Scope) {
	o.Scopes = scopes
}

// SetUID implements Object.
func (o *ObjectMeta) SetUID(uid string) {
	o.UID = uid
}

var _ ObjectList = &List[Object]{}

type List[T any] struct {
	Resource        string  `json:"resource,omitempty"`
	ResourceVersion int64   `json:"resourceVersion,omitempty"`
	Scopes          []Scope `json:"scopes,omitempty"`
	Items           []T     `json:"items" openapi:"dynamic"`
	Total           int     `json:"total"`
	Page            int     `json:"page"`
	Size            int     `json:"size"`
	Continue        string  `json:"continue,omitempty"` // Used for pagination, if set, indicates that there are more items to list
}

// GetContinue implements ObjectList.
func (b *List[T]) GetContinue() string {
	return b.Continue
}

// SetContinue implements ObjectList.
func (b *List[T]) SetContinue(continueToken string) {
	b.Continue = continueToken
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
func (b *List[T]) GetTotal() int {
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
func (b *List[T]) SetTotal(i int) {
	b.Total = i
}

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
	}
}
