package helm

import (
	"bytes"
	"encoding/json"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
)

// +k8s:openapi-gen=true
type HelmValues struct {
	Object map[string]any `json:"-"`
}

// DeepCopy indicate how to do a deep copy of Values type
func (v *HelmValues) DeepCopy() *HelmValues {
	if v == nil {
		return nil
	}
	return &HelmValues{
		// nolint: forcetypeassert
		Object: deepCopyAny(v.Object).(map[string]any),
	}
}

func deepCopyAny(in any) any {
	switch val := in.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, v := range val {
			out[k] = deepCopyAny(v)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, v := range val {
			out[i] = deepCopyAny(v)
		}
		return out
	default:
		return val
	}
}

func (v *HelmValues) UnmarshalBSON(in []byte) error {
	// check if the value is nil
	if in == nil {
		return nil
	}
	data := make(map[string]any)
	if err := bson.Unmarshal(in, &data); err != nil {
		return err
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	val := make(map[string]any)
	if err := json.Unmarshal(jsonData, &val); err != nil {
		return err
	}
	RemoveNulls(val)
	v.Object = val
	return nil
}

func (v HelmValues) MarshalBSON() ([]byte, error) {
	return bson.Marshal(v.Object)
}

func (v *HelmValues) UnmarshalJSON(in []byte) error {
	if v == nil {
		return errors.New("runtime.RawExtension: UnmarshalJSON on nil pointer")
	}
	if bytes.Equal(in, []byte("null")) {
		return nil
	}
	val := map[string]any(nil)
	if err := json.Unmarshal(in, &val); err != nil {
		return err
	}
	RemoveNulls(val)
	v.Object = val
	return nil
}

func (re HelmValues) MarshalJSON() ([]byte, error) {
	if re.Object != nil {
		return json.Marshal(re.Object)
	}
	// Value is an 'object' not null
	return []byte("{}"), nil
}

// https://github.com/helm/helm/blob/bed1a42a398b30a63a279d68cc7319ceb4618ec3/pkg/chartutil/coalesce.go#L37
// helm CoalesceValues cant handle nested null,like `{a: {b: null}}`, which want to be `{}`
func RemoveNulls(m any) {
	if m, ok := m.(map[string]any); ok {
		for k, v := range m {
			if val, ok := v.(map[string]any); ok {
				RemoveNulls(val)
				if len(val) == 0 {
					delete(m, k)
				}
				continue
			}
			if v == nil {
				delete(m, k)
				continue
			}
		}
	}
}

// b override in a
func HelmMergeMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if v, ok := v.(map[string]any); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]any); ok {
					out[k] = HelmMergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}
