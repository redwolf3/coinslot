// Package win32 is the only place Coinslot calls Win32 APIs. It reads raw facts
// (foreground window, idle time, session processes) and makes no decisions.
// It uses only the standard library's syscall package (no third-party deps).
package win32
