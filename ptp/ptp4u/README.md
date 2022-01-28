# ptp4u
Scalable PTPv2.1 two-step unicast server implementation

## Run
Normally default arguments are good for most of the cases.
However, there is a lot of room for cusomisation:
```
/usr/local/bin/ptp4u -iface eth1 -workers 100 -minsubinterval 1us -monitoringport 1234
```
This will run ptp4u on eth1 with 100 workers and allowing 1us subscriptions. Instance can be monitored on port 1234

## Monitoring
By default ptp4u runs http server serving json monitoring data. Ex:
```
$ curl localhost:8888 | jq
{
  "rx.delay_req": 0,
  "rx.signaling.announce": 0,
  "rx.signaling.delay_resp": 0,
  "rx.signaling.sync": 0,
  "subscriptions.announce": 1,
  "subscriptions.sync": 1,
  "tx.announce": 1,
  "tx.delay_resp": 0,
  "tx.follow_up": 1,
  "tx.signaling.announce": 0,
  "tx.signaling.delay_resp": 0,
  "tx.signaling.sync": 0,
  "tx.sync": 1,
  "utcoffset": 37,
  "worker.0.txtsattempts": 1
}
```
This returns manu useful metrics such as number of active subscriptions, tx/rx stats etc.

## Performance
We were able to generate and consistently support over 1M clients with synchronization frequency of 1Hz.

We used a follwing setup:
* HPE DL380 G10 single CPU system.
* Arista 7010 network switch with 2 VLANs.
* Calnex Sentinel to monitor the precision over the PTP compared to the GPS source.
* Spirent N4U to generate test clients and verify timing correctness.

Here one can see 1000513 clients registered on `ptp4u` side, where:
* 512 are constantly verified for the timing violations.
* 1 is a Calnex Sentinel to verify precision effect from the load. The vertical line represents the beginning of the test.
* 1000000 generated using traffic generator feature.
![image](https://user-images.githubusercontent.com/4749052/137388307-7d0e9e6b-df42-4d3d-bc23-b85bab458548.png)
Spirent N4U showing 512 monitoring clients are working as expected:
![image](https://user-images.githubusercontent.com/4749052/137388205-89b57751-8dca-49ab-8a6b-b43bd0382783.png)

