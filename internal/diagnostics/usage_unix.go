//go:build darwin || linux

package diagnostics

import (
	"runtime"
	"syscall"
)

func processUsage() (float64, uint64, bool, bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, 0, false, false
	}
	cpu := float64(usage.Utime.Sec+usage.Stime.Sec) + float64(usage.Utime.Usec+usage.Stime.Usec)/1e6
	rss := uint64(max(int64(0), usage.Maxrss))
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	return cpu, rss, true, true
}
