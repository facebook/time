package ntp

import (
	"fmt"
	"net"
	"time"

	syscall "golang.org/x/sys/unix"
)

// NTPEpochNanosecond is the difference between NTP and Unix epoch in NS
const NTPEpochNanosecond = int64(2208988800000000000)

// Time is converting Unix time to sec and frac NTP format
func Time(t time.Time) (seconds uint32, fracions uint32) {
	nsec := t.UnixNano() + NTPEpochNanosecond
	sec := nsec / time.Second.Nanoseconds()
	return uint32(sec), uint32((nsec - sec*time.Second.Nanoseconds()) << 32 / time.Second.Nanoseconds())
}

// Unix is converting NTP seconds and fractions into Unix time
func Unix(seconds, fractions uint32) time.Time {
	secs := int64(seconds) - NTPEpochNanosecond/time.Second.Nanoseconds()
	nanos := (int64(fractions) * time.Second.Nanoseconds()) >> 32 // convert fractional to nanos
	return time.Unix(secs, nanos)
}

// abs returns the absolute value of x
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// AvgNetworkDelay uses formula from RFC #958 to calculate average network delay
func AvgNetworkDelay(clientTransmitTime, serverReceiveTime, serverTransmitTime, clientReceiveTime time.Time) int64 {
	forwardPath := serverReceiveTime.Sub(clientTransmitTime).Nanoseconds()
	returnPath := clientReceiveTime.Sub(serverTransmitTime).Nanoseconds()

	return abs(forwardPath+returnPath) / 2
}

// CurrentRealTime returns "true" unix time after adjusting to avg network offset
func CurrentRealTime(serverTransmitTime time.Time, avgNetworkDelay int64) time.Time {
	return serverTransmitTime.Add(time.Duration(avgNetworkDelay) * time.Nanosecond)
}

// CalculateOffset returns offset between local time and "real" time
func CalculateOffset(currentRealTime, curentLocaTime time.Time) int64 {
	return currentRealTime.UnixNano() - curentLocaTime.UnixNano()
}

// EnableKernelTimestampsSocket enables socket options to read ether hardware or kernel timestamps
func EnableKernelTimestampsSocket(conn *net.UDPConn) error {
	// Get socket fd
	connfd, err := connFd(conn)
	if err != nil {
		return err
	}

	// Allow reading of hardware timestamps via socket
	if err := syscall.SetsockoptInt(connfd, syscall.SOL_SOCKET, syscall.SO_TIMESTAMPNS, 1); err != nil {
		// If we can't have hardware timestamps - use kernel timestamps
		if err := syscall.SetsockoptInt(connfd, syscall.SOL_SOCKET, syscall.SO_TIMESTAMP, 1); err != nil {
			return fmt.Errorf("failed to enable SO_TIMESTAMP: %w", err)
		}
	}
	return nil
}

// sockaddrToUDP converts syscall.Sockaddr to net.Addr
func sockaddrToUDP(sa syscall.Sockaddr) net.Addr {
	switch sa := sa.(type) {
	case *syscall.SockaddrInet4:
		return &net.UDPAddr{IP: sa.Addr[0:], Port: sa.Port}
	case *syscall.SockaddrInet6:
		return &net.UDPAddr{IP: sa.Addr[0:], Port: sa.Port}
	}
	return nil
}

// connFd returns file descriptor of a connection
func connFd(conn *net.UDPConn) (int, error) {
	connfd, err := conn.File()
	if err != nil {
		return -1, err
	}
	return int(connfd.Fd()), nil
}
