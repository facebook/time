# c4u
Config generator for ptp4u.  
It collects clock data from different sources, builds [PTP-compatible](https://github.com/facebook/time/blob/main/ptp/protocol/types.go#L287-L325) ClockClass and ClockAccuracy over specified time interval applying specified aggregation.  
Currently 2 data sources implemented:
* ts2phc
* [oscillatord](https://github.com/Orolia2s/oscillatord)

## Run
Default arguments are good for most of the cases.
However, there is a lot of room for customisation:
```
/usr/local/bin/c4u -sample 600 -path /tmp/config.txt -apply 
```
This will run c4u with sliding windown of 600 samples and generate `/tmp/config.txt` config

## Config generation
```
$ cat /etc/ptp4u.yaml
clockaccuracy: 33
clockclass: 6
draininterval: 30s
maxsubduration: 1h0m0s
metricinterval: 1m0s
minsubinterval: 1s
utcoffset: 37s
```
## Math
By default clockAccuracy will be calculated using 3 sigma rule from `ts2phc` + `oscillatord` offsets.  
ClockClass is calculated using a simple `p99` aggregation from oscillatord values. 

## Monitoring
By default c4u runs http server serving json monitoring data. Ex:
```
$ curl localhost:8889 | jq
{
  "clockaccuracy": 254,
  "clockclass": 52,
  "dataerror": 13,
  "oscillatoroffset_ns": 0,
  "phcoffset_ns": 0,
  "reload": 0,
  "utcoffset_sec": 37
}

