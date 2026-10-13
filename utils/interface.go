package utils

import (
	"encoding/json"
	"fmt"
	"strings"
)

func InterfaceString(data any) string {
	if data == nil {
		return ""
	}
	switch v := data.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		bytes, _ := json.Marshal(data)
		return string(bytes)
	}
}

func ConvertValuesToString(filters map[string]any, keys ...string) map[string]any {
	if filters == nil {
		return nil
	}
	target := map[string]struct{}{}
	for _, k := range keys {
		target[k] = struct{}{}
	}

	out := make(map[string]any, len(filters))
	for k, v := range filters {
		if len(target) == 0 {
			out[k] = toString(v)
			continue
		}
		if _, ok := target[k]; ok {
			out[k] = toString(v)
		} else {
			out[k] = v
		}
	}
	return out
}

func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case []string:
		return strings.Join(t, ",")
	case []any:
		b, _ := json.Marshal(t)
		return string(b)
	default:
		return fmt.Sprintf("%v", v)
	}
}
