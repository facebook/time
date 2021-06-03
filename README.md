# PTP [![CircleCI](https://circleci.com/gh/facebookincubator/ptp.svg?style=shield&circle-token=9254b05774162aee052aaac7773fb603d3356873)](https://circleci.com/gh/facebookincubator/ptp) [![GoDoc](https://godoc.org/github.com/facebookincubator/ptp?status.svg)](https://godoc.org/github.com/facebookincubator/ptp)

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

## ptpcheck
CLI and library to perform various PTP-related tasks, including:
* reporting stats taken from local PTP instance in JSON format
* running basic unicast client to showcase or debug PTP protocol internals
* running human-readable diagnostics for basic problems with PTP based on data from local PTP client (ptp4l).
* comparing system time with PHC time
* mapping PHC devices to network cards and vice versa

### Quick Installation
```console
go get github.com/facebookincubator/ptp/ptpcheck
```

## ptp4u
Scalable unicast PTP server.

### Quick Installation
```console
go get github.com/facebookincubator/ptp/ptp4u
```

## Simpleclient
Basic PTPv2.1 two-step unicast client implementation.

# License
PTP is licensed under Apache 2.0 as found in the [LICENSE file](LICENSE).
