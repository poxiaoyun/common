package store

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"xiaoshiai.cn/common/errors"
)

const MaxJSONPatchOperations = 10000

var _ Patch = &rawPatch{}

func RawPatch(typ PatchType, data []byte) Patch {
	return &rawPatch{typ: typ, data: data}
}

type MapMergePatch map[string]any

func (m MapMergePatch) Data(obj Object) ([]byte, error) {
	return json.Marshal(m)
}

func (m MapMergePatch) Type() PatchType {
	return PatchTypeMergePatch
}

func JSONPointerUnescape(s string) string {
	return strings.NewReplacer("~1", "/", "~0", "~").Replace(s)
}

func JSONPointerEscape(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}

type rawPatch struct {
	typ  PatchType
	data []byte
}

// Data implements Patch.
func (r *rawPatch) Data(obj Object) ([]byte, error) {
	return r.data, nil
}

// Type implements Patch.
func (r *rawPatch) Type() PatchType {
	return r.typ
}

func RawPatchBatch(typ PatchType, data []byte) PatchBatch {
	return rawBatchPatch{typ: typ, data: data}
}

type rawBatchPatch struct {
	typ  PatchType
	data []byte
}

func (p rawBatchPatch) Type() PatchType {
	return p.typ
}

func (p rawBatchPatch) Data() []byte {
	return p.data
}

type MapMergePatchBacth map[string]any

func (m MapMergePatchBacth) Data() []byte {
	val, _ := json.Marshal(m)
	return val
}

func (m MapMergePatchBacth) Type() PatchType {
	return PatchTypeMergePatch
}

func MergePatchFrom(obj Object) Patch {
	mergePatchFunc := func(originalJSON, modifiedJSON []byte, dataStruct interface{}) ([]byte, error) {
		return jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
	}
	return &mergeFromPatch{patchType: PatchTypeMergePatch, createPatch: mergePatchFunc, from: obj}
}

type mergeFromPatch struct {
	patchType   PatchType
	createPatch func(originalJSON, modifiedJSON []byte, dataStruct interface{}) ([]byte, error)
	from        Object
}

func (s *mergeFromPatch) Type() PatchType {
	return s.patchType
}

func (s *mergeFromPatch) Data(obj Object) ([]byte, error) {
	originalJSON, err := json.Marshal(s.from)
	if err != nil {
		return nil, err
	}
	modifiedJSON, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return s.createPatch(originalJSON, modifiedJSON, obj)
}

// ApplyPatch applies patch to the current object. A non-zero resourceVersion
// in a merge patch is a precondition and must match the current version.
func ApplyPatch(current Object, request Object, patch Patch) error {
	patchData, err := patch.Data(request)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	switch patch.Type() {
	case PatchTypeJSONPatch:
		return JsonPatchObject(current, patchData)
	case PatchTypeMergePatch:
		condition := struct {
			ResourceVersion *int64 `json:"resourceVersion"`
		}{}
		if err := json.Unmarshal(patchData, &condition); err != nil {
			return errors.NewBadRequest(err.Error())
		}
		if condition.ResourceVersion != nil && *condition.ResourceVersion != 0 && *condition.ResourceVersion != current.GetResourceVersion() {
			return errors.NewConflict(current.GetResource(), current.GetID(),
				fmt.Errorf("resourceVersion %d does not match", *condition.ResourceVersion))
		}
		return JsonMergePatchObject(current, patchData)
	default:
		return fmt.Errorf("unsupported patch type: %s", patch.Type())
	}
}

func JsonMergePatchObject(obj any, patch []byte) error {
	olddata, err := json.Marshal(obj)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	patchedData, err := jsonpatch.MergePatch(olddata, patch)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	if err := json.Unmarshal(patchedData, obj); err != nil {
		return errors.NewBadRequest(err.Error())
	}
	return nil
}

func JsonPatchObject(obj any, patch []byte) error {
	olddata, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	patchObj, err := jsonpatch.DecodePatch(patch)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	if len(patchObj) > MaxJSONPatchOperations {
		return errors.NewRequestEntityTooLargeError(
			fmt.Sprintf("The allowed maximum operations in a JSON patch is %d, got %d",
				MaxJSONPatchOperations, len(patchObj)))
	}
	patchedData, err := patchObj.Apply(olddata)
	if err != nil {
		if stderrors.Is(err, jsonpatch.ErrTestFailed) {
			return errors.NewCustomError(http.StatusUnprocessableEntity, errors.StatusReasonInvalid, err.Error())
		}
		return err
	}
	if err := json.Unmarshal(patchedData, obj); err != nil {
		return errors.NewBadRequest(err.Error())
	}
	return nil
}
