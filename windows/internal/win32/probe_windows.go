package win32

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/redwolf3/coinslot/windows/collector"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procEnumChildWindows         = user32.NewProc("EnumChildWindows")
	procGetLastInputInfo         = user32.NewProc("GetLastInputInfo")

	procGetTickCount                 = kernel32.NewProc("GetTickCount")
	procProcessIdToSessionId         = kernel32.NewProc("ProcessIdToSessionId")
	procQueryFullProcessImageNameW   = kernel32.NewProc("QueryFullProcessImageNameW")
	procWTSGetActiveConsoleSessionId = kernel32.NewProc("WTSGetActiveConsoleSessionId")
	procCreateMutexW                 = kernel32.NewProc("CreateMutexW")
)

const (
	processQueryLimitedInformation = 0x1000
	applicationFrameHost           = "applicationframehost.exe"
)

// Probe implements collector.Probe for the current logon session.
type Probe struct {
	SessionID uint32
}

var _ collector.Probe = (*Probe)(nil)

// NewProbe returns a probe bound to the calling process's session.
func NewProbe() (*Probe, error) {
	sid, err := SessionOf(uint32(syscall.Getpid()))
	if err != nil {
		return nil, fmt.Errorf("session id: %w", err)
	}
	return &Probe{SessionID: sid}, nil
}

// SessionOf returns the Windows session id of a process.
func SessionOf(pid uint32) (uint32, error) {
	var sid uint32
	r, _, err := procProcessIdToSessionId.Call(uintptr(pid), uintptr(unsafe.Pointer(&sid)))
	if r == 0 {
		return 0, err
	}
	return sid, nil
}

func (p *Probe) Foreground() (collector.Foreground, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return collector.Foreground{}, nil
	}
	pid := windowPID(hwnd)
	if pid == 0 {
		return collector.Foreground{}, errors.New("GetWindowThreadProcessId returned 0")
	}
	fg := collector.Foreground{PID: pid, Title: windowText(hwnd)}
	fg.Path = p.ProcessPath(pid)
	fg.Exe = filepath.Base(fg.Path)
	if fg.Path == "" {
		fg.Exe = p.exeFromSnapshot(pid)
	}
	// UWP/Store apps: the top-level window belongs to ApplicationFrameHost.exe;
	// the app's own process owns a child window.
	if strings.EqualFold(fg.Exe, applicationFrameHost) {
		if child := hostedChildPID(hwnd, pid); child != 0 {
			fg.HostExe = fg.Exe
			fg.PID = child
			fg.Path = p.ProcessPath(child)
			fg.Exe = filepath.Base(fg.Path)
			if fg.Path == "" {
				fg.Exe = p.exeFromSnapshot(child)
			}
		}
	}
	return fg, nil
}

func (p *Probe) IdleSeconds() (float64, error) {
	// LASTINPUTINFO { UINT cbSize; DWORD dwTime; }
	info := struct{ cbSize, dwTime uint32 }{cbSize: 8}
	r, _, err := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0, fmt.Errorf("GetLastInputInfo: %w", err)
	}
	now, _, _ := procGetTickCount.Call()
	// uint32 subtraction handles the 49.7-day tick wraparound.
	return float64(uint32(now)-info.dwTime) / 1000, nil
}

func (p *Probe) SessionProcesses() ([]collector.Proc, error) {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer syscall.CloseHandle(snap)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []collector.Proc
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		sid, serr := SessionOf(e.ProcessID)
		if serr != nil || sid != p.SessionID {
			continue
		}
		out = append(out, collector.Proc{PID: e.ProcessID, Exe: syscall.UTF16ToString(e.ExeFile[:])})
	}
	if !errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("Process32Next: %w", err)
	}
	return out, nil
}

func (p *Probe) ProcessPath(pid uint32) string {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

func (p *Probe) ConsoleActive() bool {
	r, _, _ := procWTSGetActiveConsoleSessionId.Call()
	return uint32(r) == p.SessionID
}

func (p *Probe) exeFromSnapshot(pid uint32) string {
	procs, err := p.SessionProcesses()
	if err != nil {
		return ""
	}
	for _, pr := range procs {
		if pr.PID == pid {
			return pr.Exe
		}
	}
	return ""
}

func windowPID(hwnd uintptr) uint32 {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func windowText(hwnd uintptr) string {
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

// EnumChildWindows callback state. syscall.NewCallback slots are a finite,
// never-freed resource, so the callback is created once.
var (
	enumMu       sync.Mutex
	enumHostPID  uint32
	enumFoundPID uint32
	enumCallback = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if pid := windowPID(hwnd); pid != 0 && pid != enumHostPID {
			enumFoundPID = pid
			return 0 // stop
		}
		return 1 // continue
	})
)

func hostedChildPID(hwnd uintptr, hostPID uint32) uint32 {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumHostPID, enumFoundPID = hostPID, 0
	procEnumChildWindows.Call(hwnd, enumCallback, 0)
	return enumFoundPID
}

// SingleInstance acquires a per-session named mutex. It returns ok=false if
// another collector already holds it. The handle is intentionally never closed;
// the OS releases it when the process exits.
func SingleInstance(name string) (ok bool, err error) {
	n, err := syscall.UTF16PtrFromString(`Local\` + name)
	if err != nil {
		return false, err
	}
	h, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(n)))
	if h == 0 {
		return false, fmt.Errorf("CreateMutexW: %w", callErr)
	}
	if errors.Is(callErr, syscall.ERROR_ALREADY_EXISTS) {
		return false, nil
	}
	return true, nil
}
