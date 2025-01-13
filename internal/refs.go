package internal

import "maps"

func Ref[k any](in k) *k {
	return &in
}

func DerefOr[k any](in *k, def k) k {
	if in == nil {
		return def
	}
	return *in
}

func DerefOrZero[k any](in *k) k {
	var def k
	return DerefOr(in, def)
}

func Or[k comparable, v any, m map[k]v](values ...m) m {
	if len(values) > 0 {
		for _, v := range values {
			if v != nil {
				return v
			}
		}
		return values[len(values)-1]
	}
	var empty m
	return empty
}

func isMap(a any) bool {
	_, b := a.(map[string]any)
	return b
}

// PatchMap performs a JSON Merge Patch as defined in https://datatracker.ietf.org/doc/html/rfc7386.
//
// This should return a new map without modifying the current or patch inputs.
// Notes:
//   - if new is nil, the output is an empty object - this allows for in-place
//   - if a key is not a map, it will be treated as scalar according to the
//     JSON Merge Patch strategy. This includes structs and slices.
func PatchMap(current map[string]interface{}, patch map[string]interface{}) map[string]interface{} {
	// small shortcut here
	if len(patch) == 0 {
		return current
	}
	out := maps.Clone(current)
	if out == nil {
		out = make(map[string]interface{})
	}
	for k, patchValue := range patch {
		if patchValue == nil {
			delete(out, k)
		} else if existingValue, ok := out[k]; ok && isMap(patchValue) {
			patchMap := patchValue.(map[string]interface{})
			if isMap(existingValue) {
				out[k] = PatchMap(existingValue.(map[string]interface{}), patchMap)
			} else {
				out[k] = PatchMap(map[string]interface{}{}, patchMap)
			}
		} else if isMap(patchValue) {
			patchMap := patchValue.(map[string]interface{})
			out[k] = PatchMap(map[string]interface{}{}, patchMap)
		} else {
			out[k] = patchValue
		}
	}
	return out
}
