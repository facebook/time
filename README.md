# NTP [![CircleCI](https://circleci.com/gh/facebookincubator/ntp.svg?style=shield&circle-token=ebd225ee8627c5cfed164ccc90da1fc124bae609)](https://circleci.com/gh/facebookincubator/ntp)

<img width="50%" 
align="right"
style="display: block; margin:40px auto;" 
src="https://raw.githubusercontent.com/leoleovich/images/master/NTP.png"/>

Collection of Facebook's NTP libraries.


## Protocol
* NTP protocol implementation
* Chrony and ntpd control protocol implementations

## Leaphash
Utility package for computing the hash value of the official leap-second.list document

## ntpcheck
CLI and library to perform various NTP-related tasks, including:
* replacement for `ntptime` and `ntpdate` commands
* human-readable diagnostics for typical problems with NTP based on data from chrony/ntpd
* server stats and peer stats taken from chrony/ntpd with output in JSON

### Quick Installation
```console
go get github.com/facebookincubator/ntp/ntpcheck
```

## Responder
Simple NTP server implementation with hardware timestamps support

### Quick Installation
```console
go get github.com/facebookincubator/ntp/responder
```


## License
ntp is licensed under Apache 2.0 as found in the [LICENSE file](LICENSE).
