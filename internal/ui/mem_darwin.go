//go:build darwin

package ui

/*
#include <libproc.h>
#include <unistd.h>

static long long kaze_footprint(void) {
	struct rusage_info_v2 ri;
	if (proc_pid_rusage(getpid(), RUSAGE_INFO_V2, (rusage_info_t *)&ri) != 0) {
		return -1;
	}
	return (long long)ri.ri_phys_footprint;
}
*/
import "C"

// processMB is the physical footprint, the number Activity Monitor shows as
// Memory: what the process has resident or compressed, native allocations
// (GPU driver, fonts, windowing) included, shared pages left out.
func processMB() (float64, bool) {
	n := C.kaze_footprint()
	if n < 0 {
		return 0, false
	}
	return float64(n) / (1 << 20), true
}
