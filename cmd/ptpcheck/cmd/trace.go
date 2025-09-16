/*
Copyright (c) Facebook, Inc. and its affiliates.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	client "github.com/facebook/time/ptp/simpleclient"
	"github.com/facebook/time/timestamp"
)

var traceRemoteServerFlag string
var traceDurationFlag time.Duration
var traceTimeoutFlag time.Duration
var traceIfaceFlag string
var traceTimestampingFlag timestamp.Timestamp

func init() {
	RootCmd.AddCommand(traceCmd)
	traceCmd.Flags().StringVarP(&traceRemoteServerFlag, "server", "S", "", "remote PTP server to connect to")
	traceCmd.Flags().StringVarP(&traceIfaceFlag, "iface", "i", "eth0", "network interface to use")
	traceCmd.Flags().VarP(&traceTimestampingFlag, "timestamping", "T", fmt.Sprintf("timestamping to use, either %q or %q", timestamp.HW, timestamp.SW))
	traceCmd.Flags().DurationVarP(&traceTimeoutFlag, "timeout", "t", 15*time.Second, "global timeout")
	traceCmd.Flags().DurationVarP(&traceDurationFlag, "duration", "d", 10*time.Second, "duration of the exchange")
}

// reportMeasurements prints all data we collected over the course of communication
func reportMeasurements(history []*client.MeasurementResult) {
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', tabwriter.AlignRight|tabwriter.Debug)
	if len(history) == 0 {
		fmt.Println("No measurements collected")
		return
	}
	fmt.Println("Collected measurements:")
	fmt.Fprintf(w, "N\t")
	for i := range history {
		fmt.Fprintf(w, "%d \t", i)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "delay\t")
	for _, d := range history {
		fmt.Fprintf(w, "%v \t", d.Delay)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "offset\t")
	for _, d := range history {
		fmt.Fprintf(w, "%v \t", d.Offset)
	}
	fmt.Fprintln(w)
	w.Flush()
}

func runTrace(cfg *client.Config) error {
	history := []*client.MeasurementResult{}
	c := client.New(cfg, func(m *client.MeasurementResult) {
		log.Infof("current numbers: delay = %v, offset = %v, clientToServerDiff = %v, serverToClientDiff = %v", m.Delay, m.Offset, m.ClientToServerDiff, m.ServerToClientDiff)
		history = append(history, m)
	})
	defer c.Close()

	err := c.Run()
	// try to report in any case, we may have collected some data before failure
	reportMeasurements(history)
	if err != nil && (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
		return err
	}
	log.Info("done")
	return nil
}

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Talk to PTP unicast server, logging every step in human-friendly form",
	Long: `Trace subcommand launches very basic PTP client to talk to specified server.
The client uses PTPv2 unicast two-step protocol, and will ask server to provide time only for specified duration.

Brief description of PTPv2 unicast two-step protocol:

First, client must negotiate the communication with the server:
1. client sends REQUEST_UNICAST_TRANSMISSION packet, asking to receive ANNOUNCE messages for certain duration.
2. server responds with GRANT_UNICAST_TRANSMISSION packet, granting ANNOUNCE messages,
and starts to periodically send such messages to client.
3. client sends REQUEST_UNICAST_TRANSMISSION packet, asking to receive SYNC messages for certain duration.
4. server responds with GRANT_UNICAST_TRANSMISSION packet, granting SYNC messages,
and starts to periodically send such messages to client.
5. client sends REQUEST_UNICAST_TRANSMISSION packet, asking to receive DELAY_RESP messages for certain duration.
6. server responds with GRANT_UNICAST_TRANSMISSION packet, granting DELAY_RESP messages.

Once negotiation is done, client and server exchange messages:

1. server sends SYNC message, which carries no timestamp in two-step protocol
2. server sends FOLLOW_UP message, which carries timestamp when SYNC was sent
3. client receives SYNC and FOLLOW_UP message, and now can perform some calculations:
	Using a timestamp when SYNC was received and timestamp in FOLLOW_UP message that marks when SYNC was actually sent,
	client can determite difference between server's clock and it's clock.
	This difference includes clock offset and network delay, thus next steps are performed.
		serverToClientDiff := syncReceivedTimestamp - syncSentTimestamp

4. on reception of SYNC message client sends DELAY_REQ packet to server, storing the  time when packet was sent.
5. on reception of DELAY_REQ message server responds with DELAY_RESP packet, which contains time when DELAY_REQ was received.
6. client receives DELAY_RESP message and can perform remaining calculations:
	Using a timestamp when DELAY_REQ was sent, and timestamp in DELAY_RESP message that marks when it was received by server,
	client can determine difference between client's clock and server clock (which again includes clock offset and network delay)
	clientToServerDiff := delayReqReceived - delayReqSent
7. with both serverToClientDiff and serverToClientDiff now known, real offset and delay are calculated using formulas:
		delay := (clientToServerDiff + serverToClientDiff)/2
		offset := (serverToClientDiff - delay)/2
	or to combine both formulas:
		offset := (serverToClientDiff - clientToServerDiff)/2

When the duration client requested to receive messages passes, server may send CANCEL_UNICAST_TRANSMISSION packet to client
to notify about this event. Clients responds with ACKNOWLEDGE_CANCEL_UNICAST_TRANSMISSION and that's all.
`,
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()

		if traceRemoteServerFlag == "" {
			log.Fatal("remote server must be specified")
		}
		if traceDurationFlag > traceTimeoutFlag {
			log.Fatal("duration must be less than timeout")
		}

		cfg := &client.Config{
			Address:      traceRemoteServerFlag,
			Iface:        traceIfaceFlag,
			Timeout:      traceTimeoutFlag,
			Duration:     traceDurationFlag,
			Timestamping: traceTimestampingFlag,
		}
		if err := runTrace(cfg); err != nil {
			log.Fatal(err)
		}

	},
}
