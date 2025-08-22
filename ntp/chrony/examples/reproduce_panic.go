//go:build ignore
// +build ignore

// This program reproduces the panic that occurs in Telegraf issue #17453
// Run with: go run reproduce_panic.go
//
// The panic occurs when binary.Write attempts to encode certain packet
// structures directly to a network connection, particularly on macOS
// with specific chrony configurations.

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

// Simplified packet structures that trigger the issue
type RequestHead struct {
	Version  uint8
	PKTType  uint8
	Res1     uint8
	Res2     uint8
	Command  uint16
	Attempt  uint16
	Sequence uint32
	Pad1     uint32
	Pad2     uint32
}

type RequestSourceStats struct {
	RequestHead
	Index int32
	EOR   int32
	// This fixed-size array causes issues with binary.Write
	data [388]uint8
}

func main() {
	fmt.Println("Attempting to reproduce chrony packet encoding panic...")
	fmt.Println("This simulates the conditions in Telegraf issue #17453")
	fmt.Println()

	// Create a mock network connection using Unix socket or TCP
	// This simulates the chrony socket connection
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	// Start a goroutine to accept the connection
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Just read to keep connection alive
		buf := make([]byte, 1024)
		conn.Read(buf)
	}()

	// Give the listener time to start
	time.Sleep(100 * time.Millisecond)

	// Connect as client
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Create a packet with the problematic structure
	packet := &RequestSourceStats{
		RequestHead: RequestHead{
			Version:  6,
			PKTType:  1,
			Command:  14, // REQ_SOURCESTATS
			Sequence: 1,
		},
		Index: 0,
		EOR:   1,
	}

	// Initialize the data array with some pattern
	// This can cause issues during encoding
	for i := range packet.data {
		packet.data[i] = uint8(i % 256)
	}

	fmt.Println("Attempting direct binary.Write to connection...")
	fmt.Println("On certain systems (particularly macOS), this may panic with:")
	fmt.Println("  'runtime error: index out of range [256] with length 256'")
	fmt.Println()

	// This is the problematic call that can panic
	err = binary.Write(conn, binary.BigEndian, packet)
	if err != nil {
		fmt.Printf("Error occurred (this is expected): %v\n", err)
		fmt.Println("\nThis error demonstrates why we need the bytes.Buffer approach.")
	} else {
		fmt.Println("No error occurred on this system.")
		fmt.Println("The panic may be system/version specific.")
	}

	fmt.Println("\nTo fix this issue, we should use bytes.Buffer:")
	fmt.Println("  var buf bytes.Buffer")
	fmt.Println("  binary.Write(&buf, binary.BigEndian, packet)")
	fmt.Println("  conn.Write(buf.Bytes())")
}
