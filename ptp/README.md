# PTP

<img width="50%"
align="right"
style="display: block; margin:40px auto;"
src="https://raw.githubusercontent.com/leoleovich/images/master/PTP.png"/>

Collection of Facebook's PTP libraries.

## Protocol
Partial implementation of PTPv2.1 (IEEE 1588-2019) protocol

## ptp4u
Scalable unicast PTP server supporting PTP and SPTP.

### Quick Installation
```console
go install github.com/facebook/time/cmd/ptp4u@latest
```

## SPTP
Scalable unicast SPTP client.

### Quick Installation
```console
go install github.com/facebook/time/cmd/sptp@latest
```

## Simpleclient
Basic PTPv2.1 two-step unicast client implementation.

## linearizability
Library to perform 'linearizability tests' - when we talk to remote GM using DelayRequest packets and compare clocks.
