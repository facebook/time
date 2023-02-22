# SPTP

Simplified Unicast PTP client

## Overview

SPTP was designed to greatly simplify the PTP unicast packet exchange, while still preserving the original PTP packet payload formats.

## Motivation

PTP was initially designed to operate in networks that support multicast. Support for unicast was added later on.
There are currently several issues that could be optimized:
* Protocol (as defined in IEEE 1588-2019) is too complex. In the context of unicast, the protocol requires a number of additional handshakes, subscriptions, timers, etc that might not be necessary (unicast negotiation, unicast discovery, duration field).
* Protocol makes any implementation fragile (multiple state machines)

## Design

![SPTP exchange](/ptp/sptp/sptp.png)

Packet exchange sequence:
1. Client sends *DELAY_REQ* effectively initiating an exchange with the Server. The Client records timestamp **T3**
2. Server records **CF_2** from *DELAY_REQ*
3. Server records the RX timestamp **T4**
4. Server sends *SYNC*. The server adds timestamp **T4** in the `originTimestamp` field and records the TX timestamp **T1**
5. Server sends *ANNOUNCE* with a TX timestamp **T1** of the *SYNC* in `originTimestamp` field and **CF_2** from *DELAY_REQ* in a `correctionField`.
6. Client records **T2** of the received *SYNC* packet, and also **CF_1**
7. Client records data from *ANNOUNCE* packet, and also **CF_2**.

As a result of this exahange the client has access to **T1, T2, T3, T4, CF_1, CF_2** to calculate mean path delay and offset metrics.
And *ANNOUNCE* message content allows traditional BMC to be used for best master selection.

This sequence is repeated based on configured interval.

As you can see, there is no state on the server, no subscription to maintain.
Client doesn't need to keep a complex state machine - all it needs it to send one packet and wait for two packets in response with some timeout.

By keeping the packets unchanged from original PTP spec we continue to enjoy PTP NICs timestamping support and network switches acting as Transparent Clocks.
The only consideration here is that *one-step* Transparent Clocks are supported.

### Messages

Only 3 message types are used:
* DELAY_REQ
* SYNC
* ANNOUNCE

#### DELAY_REQ

Packet with format described in IEEE 1588-2019 section 13.6, sent to port 319.

For server to recognise this as a start of SPTP exchange, `flagField` must have both **Unicast** and **Profile Specific 1** bits set to 1.

#### SYNC

Packet with format described in IEEE 1588-2019 section 13.6, sent to port 319 in response to *DELAY_REQ*, with `sequenceId` copied from it.

Additionally, `originTimestamp` field contains **T4** (time when server received *DELAY_REQ*)

#### ANNOUNCE

Packet with format described in IEEE 1588-2019 section 13.5, sent to port 320.

Additionally, `originTimestamp` field contains **T1** (time when server sent *SYNC*), and `correctionField` contains **CF_2** from received *DELAY_REQ* packet.


## Quick Installation
```console
go get github.com/facebook/time/cmd/sptp@latest
```

## Configuration

Example config:
```
$ cat /etc/sptp.yaml
iface: eth0
interval: 1s
timestamping: hardware
monitoringport: 4269
dscp: 35
firststepthreshold: 1s
metricsaggregationwindow: 60s
attemptstxts: 100
timeouttxts: 1ms
servers:
  "192.168.0.10": 1
  "192.168.0.11": 2
measurement:
  path_delay_filter_length: 59
  path_delay_filter: "median"
  path_delay_discard_filter_enabled: true
  path_delay_discard_below: 2us
```

## Server
Currently the only server implementation is the latest `ptp4u`.
