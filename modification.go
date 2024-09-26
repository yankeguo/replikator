package replikator

import (
	"errors"
	"time"

	"github.com/robertkrimen/otto"
	_ "github.com/robertkrimen/otto/underscore"
	"github.com/yankeguo/rg"
)

var (
	ErrScriptTimeout = errors.New("script timeout")
)

// EvaluateJavaScriptModification evaluates the javascript modification script on the src, input and output are both JSON string
func EvaluateJavaScriptModification(src string, script string) (out string, err error) {
	defer rg.Guard(&err)

	vm := otto.New()
	vm.Interrupt = make(chan func(), 1)

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-time.After(2 * time.Second):
			vm.Interrupt <- func() {
				panic(ErrScriptTimeout)
			}
		case <-done:
		}
		close(vm.Interrupt)
	}()

	if err = vm.Set("raw_resource", src); err != nil {
		return
	}
	if _, err = vm.Run("var resource = JSON.parse(raw_resource);"); err != nil {
		return
	}
	if _, err = vm.Run(script); err != nil {
		return
	}
	var val otto.Value
	if val, err = vm.Run("JSON.stringify(resource)"); err != nil {
		return
	}
	if out, err = val.ToString(); err != nil {
		return
	}
	return
}
