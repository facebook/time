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

package render

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

// Timeout bounds a command's reverse lookups: a stalled resolver must not hold
// the command open.
const Timeout = 2 * time.Second

// Table is the table every ptpcheck command prints.
func Table(w io.Writer, align []tw.Align, header ...string) *tablewriter.Table {
	table := tablewriter.NewTable(w,
		tablewriter.WithRendition(tw.Rendition{
			Symbols: tw.NewSymbols(tw.StyleASCII),
		}),
		tablewriter.WithHeaderAutoFormat(tw.Off),
	)
	table.Configure(func(cfg *tablewriter.Config) {
		cfg.Row.Alignment.PerColumn = align
	})
	table.Header(header)
	return table
}

// Addr names an address for display, keeping the address itself when there
// is no PTR or the resolver does not answer in time.
func Addr(ctx context.Context, addr string, noDNS bool) string {
	if noDNS {
		return addr
	}
	names, err := net.DefaultResolver.LookupAddr(ctx, addr)
	if err != nil || len(names) == 0 {
		return addr
	}
	return names[0]
}
