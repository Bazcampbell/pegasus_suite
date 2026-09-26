// logger/recover.go

package logger

import (
	"fmt"
	"runtime/debug"
)

// Recover logs a panic with its stack instead of letting it crash the binary; call it deferred.
func Recover(app string) {
	if r := recover(); r != nil {
		Error(Log{App: app, Message: fmt.Sprintf("recovered panic: %v\n%s", r, debug.Stack())})
	}
}
