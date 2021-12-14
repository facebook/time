# PTP

<img width="50%"
align="right"
style="display: block; margin:40px auto;"
src="https://raw.githubusercontent.com/leoleovich/images/master/PTP.png"/>

Collection of Facebook's PTP libraries.

## Protocol
Implementation of PTPv2.1 (IEEE 1588-2019) protocol

## PHC
Library to work with PTP Hardware Clock (PHC).

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

### Quick Installation
```console
go get github.com/facebook/time/ptp/ptpcheck
```

## ptp4u
Scalable unicast PTP server.

### Quick Installation
```console
go get github.com/facebook/time/ptp/ptp4u
```

## Simpleclient
Basic PTPv2.1 two-step unicast client implementation.

## oscillatord
Implementation of monitoring protocol used by Orolia [oscillatord](https://github.com/Orolia2s/oscillatord).

# License
PTP is licensed under Apache 2.0 as found in the [LICENSE file](LICENSE).
