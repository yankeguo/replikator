package replikator

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/robertkrimen/otto"
	_ "github.com/robertkrimen/otto/underscore"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ErrScriptTimeout is returned when a modification script exceeds its deadline.
var ErrScriptTimeout = errors.New("script timeout")

// EvaluateJavaScriptModification runs script against src and returns the modified JSON object.
// The script reads and mutates the global resource object. Execution is limited to javascriptTimeout.
func EvaluateJavaScriptModification(src string, script string) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			if r == ErrScriptTimeout {
				err = ErrScriptTimeout
				return
			}
			if e, ok := r.(error); ok {
				err = e
				return
			}
			err = fmt.Errorf("javascript panic: %v", r)
		}
	}()

	vm := otto.New()
	// Buffered so a timeout that fires as Run returns cannot block the timer goroutine.
	// The channel is left open: closing it makes otto call a nil interrupt function.
	vm.Interrupt = make(chan func(), 1)

	timer := time.AfterFunc(javascriptTimeout, func() {
		vm.Interrupt <- func() {
			panic(ErrScriptTimeout)
		}
	})
	defer timer.Stop()

	if err = vm.Set("raw_resource", src); err != nil {
		return "", err
	}
	if _, err = vm.Run("var resource = JSON.parse(raw_resource);"); err != nil {
		return "", err
	}
	if _, err = vm.Run(script); err != nil {
		return "", err
	}
	val, err := vm.Run("JSON.stringify(resource)")
	if err != nil {
		return "", err
	}
	out, err = val.ToString()
	if err != nil {
		return "", err
	}
	if !json.Valid([]byte(out)) {
		return "", errors.New("javascript produced invalid JSON")
	}
	return out, nil
}

func applyJSONPatch(obj *unstructured.Unstructured, patch jsonpatch.Patch) (*unstructured.Unstructured, error) {
	if len(patch) == 0 {
		return obj, nil
	}
	buf, err := obj.MarshalJSON()
	if err != nil {
		return nil, err
	}
	buf, err = patch.Apply(buf)
	if err != nil {
		return nil, err
	}
	out := &unstructured.Unstructured{}
	if err := out.UnmarshalJSON(buf); err != nil {
		return nil, err
	}
	return out, nil
}

func applyJavaScript(obj *unstructured.Unstructured, script string) (*unstructured.Unstructured, error) {
	buf, err := obj.MarshalJSON()
	if err != nil {
		return nil, err
	}
	out, err := EvaluateJavaScriptModification(string(buf), script)
	if err != nil {
		return nil, err
	}
	updated := &unstructured.Unstructured{}
	if err := updated.UnmarshalJSON([]byte(out)); err != nil {
		return nil, err
	}
	return updated, nil
}
