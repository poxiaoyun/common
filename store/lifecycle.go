package store

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/google/uuid"
	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
)

// PrepareObjectForCreate applies the server-owned metadata shared by Store
// implementations. The backend sets ResourceVersion after persistence.
func PrepareObjectForCreate(obj Object, resource string, scopes []Scope) {
	if obj.GetID() == "" {
		obj.SetID(uuid.NewString())
	}
	obj.SetUID(uuid.NewString())
	obj.SetResourceVersion(0)
	obj.SetGeneration(1)
	obj.SetCreationTimestamp(ServerTimestampNow())
	obj.SetDeletionTimestamp(nil)
	obj.SetResource(resource)
	obj.SetScopes(scopes)
}

// ValidateDeletePreconditions checks caller-supplied object identity and version.
func ValidateDeletePreconditions(obj Object, preconditions *Preconditions) error {
	if preconditions == nil {
		return nil
	}
	if preconditions.UID != nil && obj.GetUID() != *preconditions.UID {
		return errors.NewConflict(obj.GetResource(), obj.GetID(), fmt.Errorf("UID does not match"))
	}
	if preconditions.ResourceVersion != nil && obj.GetResourceVersion() != *preconditions.ResourceVersion {
		return errors.NewConflict(obj.GetResource(), obj.GetID(), fmt.Errorf("resourceVersion does not match"))
	}
	return nil
}

// ValidateDeleteRequirements checks caller-supplied object selectors.
func ValidateDeleteRequirements(obj Object, labelRequirements, fieldRequirements Requirements) error {
	if err := ValidateSelectorRequirements(labelRequirements, fieldRequirements); err != nil {
		return errors.NewBadRequest(err.Error())
	}
	unstructured, err := ToUnstructured(obj)
	if err != nil {
		return err
	}
	if !MatchLabelReqirements(obj, labelRequirements) ||
		!MatchUnstructuredFieldRequirments(unstructured, fieldRequirements) {
		return errors.NewNotFound(obj.GetResource(), obj.GetID())
	}
	return nil
}

// PrepareObjectForUpdate applies replacement or status-subresource semantics
// to desired. It returns true when removing the final finalizer completes a
// previously requested deletion.
func PrepareObjectForUpdate(current, desired Object, status bool) (bool, error) {
	currentMap, err := ObjectToMap(current)
	if err != nil {
		return false, err
	}
	desiredMap, err := ObjectToMap(desired)
	if err != nil {
		return false, err
	}
	result := desiredMap
	if status {
		result = currentMap
		if value, exists := desiredMap["status"]; exists {
			result["status"] = value
		} else {
			delete(result, "status")
		}
	} else {
		if value, exists := currentMap["status"]; exists {
			result["status"] = value
		} else {
			delete(result, "status")
		}
		for _, field := range serverOwnedUpdateFields {
			if value, exists := currentMap[field]; exists {
				result[field] = value
			} else {
				delete(result, field)
			}
		}
		generation := current.GetGeneration()
		if !reflect.DeepEqual(ObjectBusinessFields(currentMap), ObjectBusinessFields(desiredMap)) {
			generation++
		}
		result["generation"] = generation
	}
	data, err := json.Marshal(result)
	if err != nil {
		return false, err
	}
	ResetObject(desired)
	if err := json.Unmarshal(data, desired); err != nil {
		return false, err
	}
	return current.GetDeletionTimestamp() != nil && len(desired.GetFinalizers()) == 0, nil
}

// PrepareObjectForDelete applies propagation finalizers and the immutable
// deletion timestamp. It returns true when the object can be deleted now.
func PrepareObjectForDelete(obj Object, policy DeletionPropagation) bool {
	finalizers := obj.GetFinalizers()
	switch policy {
	case DeletePropagationForeground:
		if !slices.Contains(finalizers, FinalizerDeleteDependents) {
			finalizers = append(finalizers, FinalizerDeleteDependents)
		}
	case DeletePropagationOrphan:
		if !slices.Contains(finalizers, FinalizerOrphanDependents) {
			finalizers = append(finalizers, FinalizerOrphanDependents)
		}
	}
	obj.SetFinalizers(finalizers)
	if len(finalizers) == 0 {
		return true
	}
	if obj.GetDeletionTimestamp() == nil {
		deletionTimestamp := ServerTimestampNow()
		obj.SetDeletionTimestamp(&deletionTimestamp)
	}
	return false
}

// CopyObject copies the JSON representation of source into target.
func CopyObject(source, target Object) error {
	data, err := json.Marshal(source)
	if err != nil {
		return err
	}
	ResetObject(target)
	return json.Unmarshal(data, target)
}

// ResetObject clears target before decoding a complete replacement into it.
func ResetObject(target Object) {
	value := reflect.ValueOf(target).Elem()
	value.Set(reflect.Zero(value.Type()))
}

// ServerTimestampNow returns the JSON precision shared by Store backends.
func ServerTimestampNow() meta.Time {
	return meta.Time{Time: time.Now().UTC().Truncate(time.Second)}
}

// ObjectToMap returns the JSON object representation of obj.
func ObjectToMap(obj Object) (map[string]any, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ObjectBusinessFields returns fields that participate in Generation changes.
func ObjectBusinessFields(object map[string]any) map[string]any {
	result := make(map[string]any, len(object))
	for key, value := range object {
		if !slices.Contains(objectMetaFields, key) && key != "status" {
			result[key] = value
		}
	}
	return result
}

var serverOwnedUpdateFields = []string{
	"id",
	"uid",
	"resource",
	"scopes",
	"resourceVersion",
	"creationTimestamp",
	"deletionTimestamp",
}

var objectMetaFields = []string{
	"id",
	"name",
	"uid",
	"apiVersion",
	"scopes",
	"resource",
	"resourceVersion",
	"generation",
	"creationTimestamp",
	"deletionTimestamp",
	"labels",
	"annotations",
	"finalizers",
	"ownerReferences",
	"description",
}
