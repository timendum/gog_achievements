package internal

import (
	"fmt"
)

var Verbose bool

// Logf logs a message if verbose flag is set
func Logf(format string, args ...interface{}) {
	if Verbose {
		fmt.Printf("[LOG] "+format+"\n", args...)
	}
}
