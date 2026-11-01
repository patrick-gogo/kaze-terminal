package resources

import (
	"errors"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeProvider struct{}

func Native() Provider                   { return nativeProvider{} }
func (nativeProvider) Cores() int        { return int(windows.GetActiveProcessorCount(0xffff)) }
func filetime(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
func processTimes(h windows.Handle) (uint64, time.Duration, error) {
	var creation, exit, kernel, user windows.Filetime
	err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user)
	var code uint32
	if err == nil && windows.GetExitCodeProcess(h, &code) == nil && code != 259 {
		err = ErrGone
	}
	return filetime(creation), time.Duration(filetime(kernel)+filetime(user)) * 100, err
}
func (nativeProvider) List() ([]Process, bool, error) {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, false, err
	}
	defer windows.CloseHandle(h)
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(h, &e)
	out := make([]Process, 0, 256)
	for err == nil {
		if e.ProcessID != 0 {
			p := Process{PID: int(e.ProcessID), Parent: int(e.ParentProcessID)}
			ph, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
			if openErr == nil {
				p.Birth, _, _ = processTimes(ph)
				windows.CloseHandle(ph)
			}
			out = append(out, p)
		}
		if len(out) >= MaxProcesses {
			return out, true, nil
		}
		err = windows.Process32Next(h, &e)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, false, err
	}
	return out, false, nil
}

var getMemory = windows.NewLazySystemDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

// PROCESS_MEMORY_COUNTERS_EX, with pointer-sized SIZE_T fields on both architectures.
type memoryCounters struct {
	Size, Faults                                                                                uint32
	PeakWorking, Working, PeakPaged, Paged, PeakNonPaged, NonPaged, Commit, PeakCommit, Private uintptr
}

func (nativeProvider) Read(p Process) (Reading, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(p.PID))
	if err != nil {
		h, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.PID))
		if err != nil {
			return Reading{}, err
		}
	}
	defer windows.CloseHandle(h)
	birth, cpu, err := processTimes(h)
	if err != nil {
		return Reading{}, err
	}
	if birth != p.Birth {
		return Reading{}, ErrGone
	}
	r := Reading{CPU: cpu, CPUValid: true}
	m := memoryCounters{Size: uint32(unsafe.Sizeof(memoryCounters{}))}
	ok, _, _ := getMemory.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.Size))
	if ok != 0 {
		r.Bytes = uint64(m.Private)
		r.RAMValid = true
	}
	return r, nil
}
func MemoryMeaning() string { return "RAM: private committed bytes; may include paged-out memory." }
