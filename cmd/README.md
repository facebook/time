# NTP

## ntpcheck
CLI and library to perform various NTP-related tasks, including:
* replacement for `ntptime` and `ntpdate` commands
* human-readable diagnostics for typical problems with NTP based on data from chrony/ntpd
* server stats and peer stats taken from chrony/ntpd with output in JSON

### Quick Installation
```console
go install github.com/facebook/time/cmd/ntpcheck@latest
```

## NTPResponder
Simple NTP server implementation with kernel timestamps support

# PTP

## pshark
Simple tool to read pcap/pcapng captures and parse and print PTP packets from there.
Allows to test our protocol parser implementation against arbitrary tcpdump capture.
Also the code shows integration with *GoPacket* library.

## ziffy
CLI tool to triangulate datacenter switches that are not operating correctly as PTP Transparent Clocks.

## ptpcheck
CLI and library to perform various PTP-related tasks, including:
* reporting stats taken from local PTP instance in JSON format
* running basic unicast client to showcase or debug PTP protocol internals
* running human-readable diagnostics for basic problems with PTP based on data from local PTP client (ptp4l).
* comparing system time with PHC time
* mapping PHC devices to network cards and vice versa
* sync 2 PHCs

### Quick Installation
```console
go install github.com/facebook/time/cmd/ptpcheck@latest
```

## ptp4u
Scalable unicast PTP server.

### Quick Installation
```console
go install github.com/facebook/time/cmd/ptp4u@latest
```

## sptp
Scalable unicast SPTP client.

### Quick Installation
```console
go install github.com/facebook/time/cmd/sptp@latest
```

## c4u
Config generator for ptp4u.

### Quick Installation
```console
go install github.com/facebook/time/cmd/c4u@latest
```


# fbclock
## fbclock-daemon
Stateful part of fbclock (TrueTime service).

### Quick Installation
```console
go install github.com/facebook/time/cmd/fbclock-daemon@latest
```

## fbclock-bin
Simple binary written in C that uses client part of fbclock service.

### Quick Installation
```console
cd cmd/fbclock-bin
make
```

# Calnex
Command line tool for a Calnex Sentinel device
Cli Supports several basic commands such as:
* Firmware upgrade
* Configuration of the device
* Measurement data export
* Device reboot
* Device clear
* Device problem report export

### Quick Installation
```console
go install github.com/facebook/time/cmd/calnex@latest
```
