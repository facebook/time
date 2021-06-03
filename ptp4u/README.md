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
  "worker.0.load": 15,
  "worker.0.queue": 0,
  "worker.0.txtsattempts": 1
}
```
