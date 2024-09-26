package replikator

import (
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
