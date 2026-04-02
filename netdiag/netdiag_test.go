/*
Copyright (c) Facebook, Inc. and its affiliates.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package netdiag provides diagnostic tests for network interface detection
// methods across different architectures and emulation environments.
//
// This test is specifically designed to diagnose QEMU user-mode emulation
// issues on s390x where netlink syscalls fail due to incomplete byte-order
// translation of rtattr structures. It tests every available method of
// querying network interfaces to determine exactly which paths work and
// which break under emulation.
package netdiag

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func logEnvInfo(t *testing.T) {
	t.Helper()
	t.Logf("GOARCH=%s GOOS=%s", runtime.GOARCH, runtime.GOOS)

	// Check if we're under QEMU user-mode emulation
	// QEMU sets this env var, and /proc/cpuinfo may differ from GOARCH
	if v := os.Getenv("QEMU_CPU"); v != "" {
		t.Logf("QEMU_CPU=%s (QEMU user-mode detected via env)", v)
	}

	// Read /proc/cpuinfo first line for architecture hint
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		lines := strings.SplitN(string(data), "\n", 5)
		for _, line := range lines {
			if line != "" {
				t.Logf("/proc/cpuinfo: %s", line)
				break
			}
		}
	}

	// Check for forcearch/QEMU indicators
	if data, err := os.ReadFile("/proc/self/exe"); err != nil {
		t.Logf("/proc/self/exe read: %v", err)
	} else {
		_ = data
		t.Logf("/proc/self/exe readable")
	}
}

// TestMethod1_NetInterfaceByName tests the standard Go net.InterfaceByName.
// This uses netlink RTM_GETLINK under the hood.
// EXPECTED: fails under QEMU s390x with "parsenetlinkrouteattr: invalid argument"
func TestMethod1_NetInterfaceByName(t *testing.T) {
	logEnvInfo(t)

	iface, err := net.InterfaceByName("lo")
	if err != nil {
		t.Logf("FAILED: net.InterfaceByName(\"lo\"): %v", err)
		t.Logf("  error type: %T", err)
		if opErr, ok := err.(*net.OpError); ok {
			t.Logf("  OpError.Op=%s Net=%s Err=%v", opErr.Op, opErr.Net, opErr.Err)
		}
		return
	}
	t.Logf("OK: net.InterfaceByName(\"lo\"): index=%d flags=%v mtu=%d hwaddr=%v",
		iface.Index, iface.Flags, iface.MTU, iface.HardwareAddr)
}

// TestMethod2_NetInterfaces tests net.Interfaces() which lists all interfaces.
// Also uses netlink RTM_GETLINK.
// EXPECTED: fails under QEMU s390x
func TestMethod2_NetInterfaces(t *testing.T) {
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Logf("FAILED: net.Interfaces(): %v", err)
		return
	}
	t.Logf("OK: net.Interfaces() returned %d interfaces", len(ifaces))
	for _, iface := range ifaces {
		t.Logf("  %d: %s flags=%v mtu=%d", iface.Index, iface.Name, iface.Flags, iface.MTU)
	}
}

// TestMethod3_NetInterfaceAddrs tests net.InterfaceAddrs() which gets all addresses.
// Uses netlink RTM_GETADDR.
// EXPECTED: fails under QEMU s390x
func TestMethod3_NetInterfaceAddrs(t *testing.T) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Logf("FAILED: net.InterfaceAddrs(): %v", err)
		return
	}
	t.Logf("OK: net.InterfaceAddrs() returned %d addrs", len(addrs))
	for _, addr := range addrs {
		t.Logf("  %s (%s)", addr.String(), addr.Network())
	}
}

// TestMethod4_IfaceAddrs tests iface.Addrs() on a specific interface.
// Uses netlink RTM_GETADDR.
// EXPECTED: fails under QEMU s390x (if InterfaceByName succeeds, this still fails)
func TestMethod4_IfaceAddrs(t *testing.T) {
	iface, err := net.InterfaceByName("lo")
	if err != nil {
		t.Logf("SKIPPED: can't get interface: %v", err)
		return
	}
	addrs, err := iface.Addrs()
	if err != nil {
		t.Logf("FAILED: iface.Addrs(): %v", err)
		return
	}
	t.Logf("OK: lo.Addrs() returned %d addrs", len(addrs))
	for _, addr := range addrs {
		t.Logf("  %s", addr.String())
	}
}

// TestMethod5_SyscallNetlinkRIB tests the raw netlink RIB query directly.
// This is the underlying mechanism Go uses for all net.Interface* functions.
// EXPECTED: fails under QEMU s390x at ParseNetlinkRouteAttr
func TestMethod5_SyscallNetlinkRIB(t *testing.T) {
	// Step 1: Get raw netlink data
	tab, err := syscall.NetlinkRIB(syscall.RTM_GETLINK, syscall.AF_UNSPEC)
	if err != nil {
		t.Logf("FAILED: syscall.NetlinkRIB(RTM_GETLINK): %v", err)
		return
	}
	t.Logf("OK: syscall.NetlinkRIB returned %d bytes", len(tab))

	// Step 2: Parse netlink messages (outer header)
	msgs, err := syscall.ParseNetlinkMessage(tab)
	if err != nil {
		t.Logf("FAILED: syscall.ParseNetlinkMessage: %v", err)
		t.Logf("  raw first 32 bytes: %x", tab[:min(32, len(tab))])
		return
	}
	t.Logf("OK: ParseNetlinkMessage returned %d messages", len(msgs))

	// Step 3: Parse route attributes (inner attributes - this is where it breaks)
	for i, msg := range msgs {
		if msg.Header.Type == syscall.NLMSG_DONE {
			continue
		}
		attrs, err := syscall.ParseNetlinkRouteAttr(&msg)
		if err != nil {
			t.Logf("FAILED: ParseNetlinkRouteAttr on msg[%d] (type=%d, len=%d): %v",
				i, msg.Header.Type, msg.Header.Len, err)
			if len(msg.Data) >= 20 {
				t.Logf("  msg.Data first 20 bytes: %x", msg.Data[:20])
			}
			return
		}
		if i == 0 {
			t.Logf("OK: ParseNetlinkRouteAttr on msg[0] returned %d attrs", len(attrs))
			for j, attr := range attrs {
				if j < 5 {
					t.Logf("  attr[%d]: type=%d len=%d", j, attr.Attr.Type, attr.Attr.Len)
				}
			}
		}
	}
	t.Logf("OK: all %d messages parsed successfully", len(msgs))
}

// TestMethod6_ReadSysfs reads interface info from /sys/class/net/.
// This does NOT use netlink - it reads sysfs directly.
// EXPECTED: works everywhere including QEMU s390x
func TestMethod6_ReadSysfs(t *testing.T) {
	sysNetDir := "/sys/class/net"
	entries, err := os.ReadDir(sysNetDir)
	if err != nil {
		t.Logf("FAILED: ReadDir(%s): %v", sysNetDir, err)
		return
	}
	t.Logf("OK: /sys/class/net has %d entries", len(entries))

	for _, entry := range entries {
		name := entry.Name()
		// Read key properties from sysfs
		readSysfsFile := func(prop string) string {
			data, err := os.ReadFile(filepath.Join(sysNetDir, name, prop))
			if err != nil {
				return fmt.Sprintf("error: %v", err)
			}
			return strings.TrimSpace(string(data))
		}

		t.Logf("  %s: operstate=%s address=%s mtu=%s",
			name,
			readSysfsFile("operstate"),
			readSysfsFile("address"),
			readSysfsFile("mtu"),
		)
	}
}

// TestMethod7_ReadProcNet reads interface info from /proc/net/dev.
// This does NOT use netlink - it reads procfs directly.
// EXPECTED: works everywhere including QEMU s390x
func TestMethod7_ReadProcNet(t *testing.T) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		t.Logf("FAILED: ReadFile(/proc/net/dev): %v", err)
		return
	}
	lines := strings.Split(string(data), "\n")
	t.Logf("OK: /proc/net/dev has %d lines", len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			t.Logf("  %s", line)
		}
	}
}

// TestMethod8_ReadProcNetIfInet6 reads IPv6 addresses from /proc/net/if_inet6.
// No netlink involved.
// EXPECTED: works everywhere including QEMU s390x
func TestMethod8_ReadProcNetIfInet6(t *testing.T) {
	data, err := os.ReadFile("/proc/net/if_inet6")
	if err != nil {
		t.Logf("FAILED: ReadFile(/proc/net/if_inet6): %v (may not exist if IPv6 disabled)", err)
		return
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	t.Logf("OK: /proc/net/if_inet6 has %d entries", len(lines))
	for _, line := range lines {
		t.Logf("  %s", line)
	}
}

// TestMethod9_NetDial tests if basic TCP/UDP networking works (no netlink needed).
// EXPECTED: works everywhere including QEMU s390x
func TestMethod9_NetDial(t *testing.T) {
	// Listen on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Logf("FAILED: net.Listen: %v", err)
		return
	}
	defer ln.Close()
	t.Logf("OK: net.Listen on %s", ln.Addr().String())

	// Try UDP too
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Logf("FAILED: net.ListenPacket(udp): %v", err)
		return
	}
	defer conn.Close()
	t.Logf("OK: net.ListenPacket(udp) on %s", conn.LocalAddr().String())
}

// TestMethod10_NetLookupAddr tests DNS/address resolution which may or may not
// use netlink depending on the resolver path.
// EXPECTED: likely works under QEMU (uses /etc/hosts or DNS, not netlink)
func TestMethod10_NetLookupAddr(t *testing.T) {
	addrs, err := net.LookupHost("localhost")
	if err != nil {
		t.Logf("FAILED: net.LookupHost(\"localhost\"): %v", err)
		return
	}
	t.Logf("OK: net.LookupHost(\"localhost\") = %v", addrs)
}

// TestSummary runs all methods and prints a summary table.
func TestSummary(t *testing.T) {
	logEnvInfo(t)

	type result struct {
		method string
		path   string
		err    error
	}

	var results []result

	// Method 1: net.InterfaceByName
	_, err := net.InterfaceByName("lo")
	results = append(results, result{"net.InterfaceByName", "netlink RTM_GETLINK", err})

	// Method 2: net.Interfaces
	_, err = net.Interfaces()
	results = append(results, result{"net.Interfaces", "netlink RTM_GETLINK", err})

	// Method 3: net.InterfaceAddrs
	_, err = net.InterfaceAddrs()
	results = append(results, result{"net.InterfaceAddrs", "netlink RTM_GETADDR", err})

	// Method 4: sysfs
	_, err = os.ReadDir("/sys/class/net")
	results = append(results, result{"os.ReadDir(/sys/class/net)", "sysfs (no netlink)", err})

	// Method 5: /proc/net/dev
	_, err = os.ReadFile("/proc/net/dev")
	results = append(results, result{"os.ReadFile(/proc/net/dev)", "procfs (no netlink)", err})

	// Method 6: /proc/net/if_inet6
	_, err = os.ReadFile("/proc/net/if_inet6")
	results = append(results, result{"os.ReadFile(/proc/net/if_inet6)", "procfs (no netlink)", err})

	// Method 7: raw netlink + ParseNetlinkMessage
	tab, nlErr := syscall.NetlinkRIB(syscall.RTM_GETLINK, syscall.AF_UNSPEC)
	if nlErr != nil {
		results = append(results, result{"syscall.NetlinkRIB", "netlink raw", nlErr})
	} else {
		msgs, parseErr := syscall.ParseNetlinkMessage(tab)
		if parseErr != nil {
			results = append(results, result{"ParseNetlinkMessage", "netlink raw", parseErr})
		} else {
			results = append(results, result{"ParseNetlinkMessage", "netlink raw (outer header)", nil})
			// Try parsing attrs on first non-DONE message
			for _, msg := range msgs {
				if msg.Header.Type == syscall.NLMSG_DONE {
					continue
				}
				_, attrErr := syscall.ParseNetlinkRouteAttr(&msg)
				results = append(results, result{"ParseNetlinkRouteAttr", "netlink raw (inner rtattr)", attrErr})
				break
			}
		}
	}

	// Method 8: TCP listen
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if ln != nil {
		ln.Close()
	}
	results = append(results, result{"net.Listen(tcp)", "socket (no netlink)", err})

	// Print summary
	t.Logf("")
	t.Logf("=== DIAGNOSTIC SUMMARY (GOARCH=%s) ===", runtime.GOARCH)
	t.Logf("%-35s %-30s %s", "METHOD", "PATH", "RESULT")
	t.Logf("%-35s %-30s %s", strings.Repeat("-", 35), strings.Repeat("-", 30), strings.Repeat("-", 30))
	for _, r := range results {
		status := "OK"
		if r.err != nil {
			status = fmt.Sprintf("FAIL: %v", r.err)
		}
		t.Logf("%-35s %-30s %s", r.method, r.path, status)
	}
	t.Logf("")
	t.Logf("If netlink methods FAIL but sysfs/procfs methods OK, this confirms QEMU user-mode")
	t.Logf("emulation issue: kernel returns netlink data in host byte order (little-endian)")
	t.Logf("but the emulated binary reads it as big-endian.")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
