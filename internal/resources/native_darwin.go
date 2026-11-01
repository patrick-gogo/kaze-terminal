package resources

/*
#include <libproc.h>
#include <mach/mach_time.h>
#include <sys/proc_info.h>
#include <sys/resource.h>
#include <sys/sysctl.h>
#include <errno.h>
static int kaze_process(int pid, struct proc_bsdinfo *b) {
 return proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, b, sizeof(*b));
}
static int kaze_usage(int pid, struct rusage_info_v2 *r) {
 return proc_pid_rusage(pid, RUSAGE_INFO_V2, (rusage_info_t *)r);
}
static int kaze_cores(void) {
 int n=0;size_t len=sizeof(n);
 if(sysctlbyname("hw.logicalcpu", &n, &len, NULL, 0)!=0) return 0;
 return n;
}
*/
import "C"
import (
	"errors"
	"time"
	"unsafe"
)

type nativeProvider struct{ numer, denom uint64 }

func Native() Provider {
	var t C.mach_timebase_info_data_t
	if C.mach_timebase_info(&t) != 0 {
		return nativeProvider{}
	}
	return nativeProvider{uint64(t.numer), uint64(t.denom)}
}
func (nativeProvider) Cores() int { return int(C.kaze_cores()) }
func (nativeProvider) List() ([]Process, bool, error) {
	// A fixed bounded buffer avoids repeated enumeration and runaway allocation.
	ids := make([]C.int, MaxProcesses)
	n := int(C.proc_listallpids(unsafe.Pointer(&ids[0]), C.int(len(ids)*4)))
	if n <= 0 {
		return nil, false, errors.New("process list unavailable")
	}
	partial := n >= len(ids)
	n = min(n, len(ids))
	out := make([]Process, 0, n)
	for _, pid := range ids[:n] {
		if pid <= 0 {
			continue
		}
		var b C.struct_proc_bsdinfo
		if C.kaze_process(pid, &b) != C.int(unsafe.Sizeof(b)) {
			out = append(out, Process{PID: int(pid)})
			continue
		}
		out = append(out, Process{PID: int(pid), Parent: int(b.pbi_ppid), Birth: uint64(b.pbi_start_tvsec)*1000000 + uint64(b.pbi_start_tvusec)})
	}
	return out, partial, nil
}
func (n nativeProvider) Read(p Process) (Reading, error) {
	var b C.struct_proc_bsdinfo
	if C.kaze_process(C.int(p.PID), &b) != C.int(unsafe.Sizeof(b)) {
		return Reading{}, ErrGone
	}
	birth := uint64(b.pbi_start_tvsec)*1000000 + uint64(b.pbi_start_tvusec)
	if birth != p.Birth {
		return Reading{}, ErrGone
	}
	var r C.struct_rusage_info_v2
	if C.kaze_usage(C.int(p.PID), &r) != 0 {
		return Reading{}, errors.New("process usage unavailable")
	}
	at := time.Now() // timestamp the CPU counter before identity revalidation
	// Recheck creation identity after the usage call to reject PID reuse.
	if C.kaze_process(C.int(p.PID), &b) != C.int(unsafe.Sizeof(b)) || uint64(b.pbi_start_tvsec)*1000000+uint64(b.pbi_start_tvusec) != p.Birth {
		return Reading{}, ErrGone
	}
	// rusage CPU counters are Mach absolute ticks, not nanoseconds on Apple Silicon.
	ticks := uint64(r.ri_user_time) + uint64(r.ri_system_time)
	var nanos uint64
	if n.denom != 0 {
		nanos = ticks/n.denom*n.numer + (ticks%n.denom)*n.numer/n.denom
	}
	return Reading{At: at, CPU: time.Duration(nanos), Bytes: uint64(r.ri_phys_footprint), CPUValid: n.denom != 0, RAMValid: true}, nil
}
func MemoryMeaning() string { return "RAM: physical footprint, including compressed memory." }
