package telemetry

import "runtime"

func currentGoroutines() int {
	return runtime.NumGoroutine()
}
