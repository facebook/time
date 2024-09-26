# NTP

<img width="50%"
align="right"
style="display: block; margin:40px auto;"
src="https://raw.githubusercontent.com/leoleovich/images/master/NTP.png"/>

Collection of Facebook's NTP libraries.

## Protocol
Basic NTPv4 protocol implementation

ntp.go
 - Time function converts Time object from Unix time to NTP format
 - Unix function converts Time object from NTP format to Unix format
 - Offset function calculates difference between clock times for client and server
 - RoundTripDelay function calculates delay during transmission between server and client
 - CorrectTime function calculates the real time for client based on the received time and a given clock offset

packet.go
 - in charge of converting packet data into byte data, and vice versa
 - ReadNTPPacket function takes in raw byte data from a given connection, converts data into an NTP packet and
   returns the data packet with original connection's remote address

## Chrony
Chrony control protocol implementation

## Control
ntpd control protocol implementation

## Responder
Simple NTP server implementation with hardware timestamps support

## shm
NTPSHM library

### Quick Installation
```console
go install github.com/facebook/time/cmd/ntpresponder@latest
```
