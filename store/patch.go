package store

import (
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	"xiaoshiai.cn/common/errors"
)

const MaxJSONPatchOperations = 10000

var _ Patch = &rawPatch{}

func RawPatch(typ PatchType, data []byte) Patch {
	return &rawPatch{typ: typ, data: data}
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

func ApplyPatch(to Object, from Object, patch Patch) error {
	patchtype := patch.Type()
	patchdata, err := patch.Data(from)
	if err != nil {
		return errors.NewBadRequest(err.Error())
	}
	switch patchtype {
	case PatchTypeJSONPatch:
		return JsonPatchObject(to, patchdata)
	case PatchTypeMergePatch:
		return JsonMergePatchObject(to, patchdata)
	default:
		return fmt.Errorf("unsupported patch type: %s", patchtype)
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
		return err
	}
	if err := json.Unmarshal(patchedData, obj); err != nil {
		return errors.NewBadRequest(err.Error())
	}
	return nil
}
