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
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
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

	// Check binfmt_misc for QEMU registration
	binfmtPath := "/proc/sys/fs/binfmt_misc/qemu-" + runtime.GOARCH
	if data, err := os.ReadFile(binfmtPath); err == nil {
		t.Logf("binfmt_misc (%s):\n%s", binfmtPath, string(data))
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "interpreter ") {
				interp := strings.TrimPrefix(line, "interpreter ")
				if out, err := exec.Command(interp, "--version").CombinedOutput(); err == nil {
					t.Logf("QEMU version: %s", strings.TrimSpace(string(out)))
				} else {
					t.Logf("QEMU interpreter %s found but --version failed: %v", interp, err)
				}
			}
		}
	} else {
		t.Logf("binfmt_misc: %s not found (not running under QEMU, or different registration name)", binfmtPath)
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
		var opErr *net.OpError
		if errors.As(err, &opErr) {
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
// It dumps raw bytes to help diagnose QEMU translation issues.
// EXPECTED: fails under QEMU s390x at ParseNetlinkRouteAttr
func TestMethod5_SyscallNetlinkRIB(t *testing.T) {
	// Note: Go's NetlinkRIB uses Recvfrom (not recvmsg), so QEMU's
	// fd_trans callback for recvfrom is the relevant code path.

	// Test both RTM_GETLINK (fails under QEMU) and RTM_GETADDR (works)
	for _, tc := range []struct {
		name  string
		proto int
	}{
		{"RTM_GETLINK", syscall.RTM_GETLINK},
		{"RTM_GETADDR", syscall.RTM_GETADDR},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Get raw netlink data
			tab, err := syscall.NetlinkRIB(tc.proto, syscall.AF_UNSPEC)
			if err != nil {
				t.Logf("FAILED: syscall.NetlinkRIB(%s): %v", tc.name, err)
				return
			}
			t.Logf("OK: NetlinkRIB(%s) returned %d bytes", tc.name, len(tab))

			// Dump first 64 bytes of raw response
			dumpLen := 64
			if len(tab) < dumpLen {
				dumpLen = len(tab)
			}
			t.Logf("  raw first %d bytes: %x", dumpLen, tab[:dumpLen])

			// Step 2: Parse outer netlink message header
			msgs, err := syscall.ParseNetlinkMessage(tab)
			if err != nil {
				t.Logf("FAILED: ParseNetlinkMessage(%s): %v", tc.name, err)
				return
			}
			t.Logf("OK: ParseNetlinkMessage returned %d messages", len(msgs))

			// Step 3: For each message, dump details and try parsing rtattr
			for i, msg := range msgs {
				if msg.Header.Type == syscall.NLMSG_DONE {
					continue
				}
				t.Logf("  msg[%d]: Type=%d Len=%d Flags=0x%x Seq=%d",
					i, msg.Header.Type, msg.Header.Len, msg.Header.Flags, msg.Header.Seq)

				// Dump the message data (contains ifinfomsg/ifaddrmsg + rtattrs)
				dataDumpLen := 64
				if len(msg.Data) < dataDumpLen {
					dataDumpLen = len(msg.Data)
				}
				if dataDumpLen > 0 {
					t.Logf("  msg[%d].Data first %d bytes: %x", i, dataDumpLen, msg.Data[:dataDumpLen])
				}

				// Identify the rtattr region offset based on message type
				var rtaOffset int
				switch msg.Header.Type {
				case syscall.RTM_NEWLINK, syscall.RTM_DELLINK:
					rtaOffset = syscall.SizeofIfInfomsg // 16 bytes
					t.Logf("  msg[%d]: RTM_*LINK, rtattr starts at offset %d (after IfInfomsg)", i, rtaOffset)
				case syscall.RTM_NEWADDR, syscall.RTM_DELADDR:
					rtaOffset = syscall.SizeofIfAddrmsg // 8 bytes
					t.Logf("  msg[%d]: RTM_*ADDR, rtattr starts at offset %d (after IfAddrmsg)", i, rtaOffset)
				}

				// Dump the first rtattr bytes specifically
				if len(msg.Data) > rtaOffset+4 {
					rtaBytes := msg.Data[rtaOffset:]
					rtaDumpLen := 32
					if len(rtaBytes) < rtaDumpLen {
						rtaDumpLen = len(rtaBytes)
					}
					t.Logf("  msg[%d] rtattr region (%d bytes): %x", i, len(rtaBytes), rtaBytes[:rtaDumpLen])
					// Show first rtattr Len/Type as raw uint16 in both byte orders
					if len(rtaBytes) >= 4 {
						t.Logf("  msg[%d] first rtattr bytes [0:4]: %02x %02x %02x %02x", i,
							rtaBytes[0], rtaBytes[1], rtaBytes[2], rtaBytes[3])
						leLEN := uint16(rtaBytes[0]) | uint16(rtaBytes[1])<<8
						beLEN := uint16(rtaBytes[0])<<8 | uint16(rtaBytes[1])
						t.Logf("  msg[%d] rtattr.Len as LE: %d, as BE: %d (SizeofRtAttr=%d)", i,
							leLEN, beLEN, syscall.SizeofRtAttr)
					}
				}

				// Try parsing route attributes
				attrs, err := syscall.ParseNetlinkRouteAttr(&msg)
				if err != nil {
					t.Logf("  FAILED: ParseNetlinkRouteAttr msg[%d]: %v", i, err)
				} else {
					t.Logf("  OK: ParseNetlinkRouteAttr msg[%d] returned %d attrs", i, len(attrs))
					for j, attr := range attrs {
						if j < 3 {
							t.Logf("    attr[%d]: Type=%d Len=%d ValueLen=%d", j, attr.Attr.Type, attr.Attr.Len, len(attr.Value))
						}
					}
				}

				// Only dump first 2 messages to keep output manageable
				if i >= 1 {
					t.Logf("  (skipping remaining %d messages)", len(msgs)-i-1)
					break
				}
			}
		})
	}
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

	// Print summary using fmt.Println so it always appears in build logs
	// (t.Logf is only shown for failing tests or with -v flag)
	// We also call t.Errorf if netlink failures are detected, which forces
	// Go to print all t.Log output for this test.
	t.Log("")
	t.Logf("=== NETDIAG DIAGNOSTIC SUMMARY (GOARCH=%s) ===", runtime.GOARCH)
	t.Logf("%-35s %-30s %s", "METHOD", "PATH", "RESULT")
	t.Logf("%-35s %-30s %s", strings.Repeat("-", 35), strings.Repeat("-", 30), strings.Repeat("-", 30))
	hasNetlinkFailure := false
	allNonNetlinkOK := true
	for _, r := range results {
		status := "OK"
		if r.err != nil {
			status = fmt.Sprintf("FAIL: %v", r.err)
			if strings.Contains(r.path, "netlink") {
				hasNetlinkFailure = true
			} else {
				allNonNetlinkOK = false
			}
		}
		t.Logf("%-35s %-30s %s", r.method, r.path, status)
	}
	t.Log("")
	if hasNetlinkFailure && allNonNetlinkOK {
		t.Log("DIAGNOSIS: Netlink methods FAIL but sysfs/procfs methods OK.")
		t.Log("This confirms QEMU user-mode emulation issue: kernel returns")
		t.Log("netlink data in host byte order (little-endian) but the emulated")
		t.Log("s390x binary reads it as big-endian.")
		t.Error("QEMU netlink emulation issue detected (this failure is expected under QEMU)")
	} else if !hasNetlinkFailure {
		t.Log("DIAGNOSIS: All methods OK. This is a native build (no QEMU issue).")
	} else {
		t.Log("DIAGNOSIS: Mixed failures across netlink and non-netlink methods.")
		t.Log("This may indicate a different issue than QEMU emulation.")
		t.Error("Unexpected failure pattern detected")
	}
	t.Log("=== END NETDIAG ===")
}
