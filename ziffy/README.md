# ziffy
CLI tool to triangulate datacenter switches that are not operating correctly as PTP Transparent Clocks.

### How to sweep the network topology?

Ziffy sends PTP SYNC/DELAY\_REQ packets between two hosts, a Ziffy sender and a Ziffy receiver in order to get data about the topology. It supports sending packets from a range of source ports to encourage hashing of traffic over multiple paths. In case the hashing is done using only destination IP and source IP, Ziffy can target multiple IPs in the same /64 prefix as the destination.

### How to determine if a switch is operating as PTP TC?

In order to determine if a switch is a PTP Transparent Clock, Ziffy sends IPv6 PTP SYNC/DELAY\_REQ packets with the same (srcIP, srcPort, destIP, destPort) tuple but with consecutive Hop Limit. When a PTP packet is dropped by a switch, a ICMPv6 Hop limit exceeded packet containing the entire PTP packet is returned to the sender.

Consider that `Switch3` was hit using `HopLimit=3` and `Switch4` was hit using `HopLimit=4`. If `(Switch4.CorrectionField - Switch3.CorrectionField < threshold)` then `Switch3` did not modify the `CorrectionField`, so it is not a Transparent Clock.

## Run
Run Ziffy in receiver mode on a host, and Ziffy in sender mode on another host.

```
sudo ziffy -mode receiver
```
```
sudo ziffy -mode sender -addr <<receiver_hostname>> -maxhop 6 -portcount 80 -ipcount 5 -sp 31000
```

This will send 80 PTP SYNC packets to `<<receiver_hostname>>` from source port range 31000-31079 with max hop count of 6 and min hop count of 1, sweeping 5 more addresses in target network prefix. Total flows 480.

## Requirements
* IPv6
* sudo - needed because
    * ziffy listens for packets even if the port is already used by another process, using a BPF filter 
    * ziffy decodes raw packets
    * minimal capabilities to run ziffy: `cap_net_raw=eip`.
* pcap library - used to build and run ziffy.
