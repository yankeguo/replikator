package replikator

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// versionPattern matches Kubernetes API versions such as v1 and v1beta1.
var versionPattern = regexp.MustCompile(`^v[0-9]`)

// ParseGroupVersionResource parses a canonical resource string.
//
// Accepted forms are "secrets", "apps/deployments", "v1beta1/ingresses",
// and "networking.k8s.io/v1/ingresses". A missing version means v1.
func ParseGroupVersionResource(s string) (schema.GroupVersionResource, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return schema.GroupVersionResource{}, errors.New("resource is required")
	}

	parts := strings.Split(s, "/")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return schema.GroupVersionResource{}, fmt.Errorf("invalid resource %q", s)
		}
	}

	var res schema.GroupVersionResource
	switch len(parts) {
	case 1:
		res.Version = "v1"
		res.Resource = parts[0]
	case 2:
		if versionPattern.MatchString(parts[0]) {
			res.Version = parts[0]
		} else {
			res.Group = parts[0]
			res.Version = "v1"
		}
		res.Resource = parts[1]
	case 3:
		if !versionPattern.MatchString(parts[1]) {
			return schema.GroupVersionResource{}, fmt.Errorf("invalid resource %q: version %q", s, parts[1])
		}
		res.Group = parts[0]
		res.Version = parts[1]
		res.Resource = parts[2]
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("invalid resource %q", s)
	}
	return res, nil
}

// RetrieveMetadataName returns metadata.name from a Kubernetes object.
func RetrieveMetadataName(obj any) (string, error) {
	if obj == nil {
		return "", errors.New("metadata.name not found")
	}
	if ro, ok := obj.(runtime.Object); ok {
		accessor, err := meta.Accessor(ro)
		if err == nil {
			if name := accessor.GetName(); name != "" {
				return name, nil
			}
		}
	}
	if m, ok := obj.(map[string]any); ok {
		if metadata, ok := m["metadata"].(map[string]any); ok {
			if name, ok := metadata["name"].(string); ok && name != "" {
				return name, nil
			}
		}
	}
	return "", errors.New("metadata.name not found")
}
