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

package pdelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Results is a list of Result exchanged over the sptp /ping handler
type Results []*Result

// resultJSON mirrors Result with the error rendered as a string. Every other
// field marshals as itself: netip.Addr and time.Time both round-trip exactly,
// including their zero values.
type resultJSON struct {
	*resultAlias
	Error string `json:"error,omitempty"`
}

// resultAlias drops the marshaller so embedding it does not recurse
type resultAlias Result

// MarshalJSON renders the measurement with Error as a string
func (r Result) MarshalJSON() ([]byte, error) {
	out := resultJSON{resultAlias: (*resultAlias)(&r)}
	if r.Error != nil {
		out.Error = r.Error.Error()
	}
	return json.Marshal(out)
}

// UnmarshalJSON restores the measurement, turning the error string back into an error
func (r *Result) UnmarshalJSON(b []byte) error {
	// decode into a zero value: decoding through the receiver would let fields
	// absent from the payload keep whatever the reused Result already held
	var fresh Result
	in := resultJSON{resultAlias: (*resultAlias)(&fresh)}
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	// the sentinel has to come back by identity: rebuilding it from the message
	// would make errors.Is false for every Result that crossed the wire, which
	// in production is all of them
	if in.Error == ErrIncompleteResponse.Error() {
		fresh.Error = ErrIncompleteResponse
	} else if in.Error != "" {
		fresh.Error = errors.New(in.Error)
	}
	*r = fresh
	return nil
}

// FetchPing asks sptp at baseURL to ping target and returns the measurements.
// The timeout must exceed the timeout sptp itself uses to collect responses.
func FetchPing(ctx context.Context, baseURL string, target string, timeout time.Duration) (Results, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing sptp url %q: %w", baseURL, err)
	}
	u.Path = "/ping"
	u.RawQuery = url.Values{"target": []string{target}}.Encode()

	c := http.Client{
		Timeout: timeout,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("sptp ping %s: %w", target, err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sptp ping %s: %w", target, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading sptp ping response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sptp ping %s: %s: %s", target, resp.Status, b)
	}

	var res Results
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("decoding sptp ping response: %w", err)
	}
	// a top-level null decodes without error but is not the documented array
	if res == nil {
		return nil, fmt.Errorf("sptp ping %s: response is not a JSON array", target)
	}
	for _, r := range res {
		if r == nil {
			return nil, fmt.Errorf("sptp ping %s: response contains a null entry", target)
		}
	}
	return res, nil
}
