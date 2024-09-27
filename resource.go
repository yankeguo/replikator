package replikator

import (
	"encoding/json"
	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ParseGroupVersionResource parse a string to GroupVersionResource
func ParseGroupVersionResource(s string) (res schema.GroupVersionResource, err error) {
	splits := strings.Split(s, "/")
	switch len(splits) {
	case 1:
		res.Version = "v1"
		res.Resource = splits[0]
	case 2:
		if strings.HasPrefix(splits[0], "v") {
			res.Version = splits[0]
		} else {
			res.Group = splits[0]
			res.Version = "v1"
		}
		res.Resource = splits[1]
	case 3:
		res.Group = splits[0]
		res.Version = splits[1]
		res.Resource = splits[2]
	default:
		err = errors.New("invalid resource: " + s)
	}
	return
}

// RetrieveMetadataName retrieve metadata.name from an object
func RetrieveMetadataName(obj any) (name string, err error) {
	var buf []byte
	if buf, err = json.Marshal(obj); err != nil {
		return
	}
	var m map[string]interface{}
	if err = json.Unmarshal(buf, &m); err != nil {
		return
	}
	if metadata, ok := m["metadata"].(map[string]interface{}); ok {
		if n, ok := metadata["name"].(string); ok {
			name = n
			return
		}
	}
	err = errors.New("metadata.name not found")
	return
}
