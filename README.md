# Time

[![Build Status](https://img.shields.io/github/workflow/status/facebook/time/test/main)](https://github.com/facebook/time/actions?query=branch%3Amain)
[![codecov](https://codecov.io/gh/facebook/time/branch/main/graph/badge.svg?token=QC44PEpHRi)](https://codecov.io/gh/facebook/time)
[![Go Report Card](https://goreportcard.com/badge/github.com/facebook/time)](https://goreportcard.com/report/github.com/facebook/time)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/facebook/time)
[![GoDoc](https://pkg.go.dev/badge/github.com/facebook/time?status.svg)](https://pkg.go.dev/github.com/facebook/time?tab=doc)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

# Contents

- [Documentation](#Documentation)
- [License](#License)

## Documentation

Collection of Meta's Time Libraries such as NTP and PTP

### cmd
All executables provided by this repo.

### NTP
NTP-specific libraries, including protocol implementation.

### PTP
PTP-specific libraries, including protocol implementation.

### Leaphash
Utility package for computing the hash value of the official leap-second.list document

### leapsectz
Utility package for obtaining leap second information from the system timezone database

### PHC
Library to work with PTP Hardware Clock (PHC).

### Timestamp
Library to work with NIC hardware/software timestamps.

### oscillatord
Implementation of monitoring protocol used by Orolia [oscillatord](https://github.com/Orolia2s/oscillatord).

### Calnex
Command line tool and library for a Calnex Sentinel device.

### fbclock
Client C library and Go daemon to provide TrueTime API based on PTP time.

# License
time is licensed under Apache 2.0 as found in the [LICENSE file](LICENSE).
