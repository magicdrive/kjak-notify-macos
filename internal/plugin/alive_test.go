package plugin

import (
	"strconv"
	"syscall"
	"time"
)

// alive reports whether the process pid still runs (after a short grace for
// the kill to land).
func alive(pid string) bool {
	n, err := strconv.Atoi(pid)
	if err != nil || n <= 0 {
		return false
	}
	for range 20 {
		if syscall.Kill(n, 0) != nil {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true
}
