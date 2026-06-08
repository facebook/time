# sa53

Tools for the Microchip SA53 atomic clock on Celestica time cards.

Built as a single binary `sa53` (installed to `/usr/local/bin/sa53` via the
`fb-sa53` RPM) using cobra subcommands.

## Subcommands

### firmware

Read or upgrade SA53 firmware.

```
sudo sa53 firmware --check                       # print version as JSON
sudo sa53 firmware --fw firmware.hex --upgrade   # apply an upgrade
sudo sa53 firmware --fw firmware.hex --upgrade --force
```

Old single-dash flags (`-upgrade`) are gone; cobra requires the subcommand and `--`
long flags.

### poll

Capture SA53 telemetry to CSV for vendor support handoff.

Canonical vendor handoff invocation:

```
# stop chef (so it doesn't restart oscillatord), then stop oscillatord
systemctl stop oscillatord
sa53 poll --duration 5m --interval 100ms --out /tmp/sa53.csv --raw /tmp/sa53.raw
systemctl start oscillatord
# then start chef again per the standard procedure
```

One-shot probe: `sudo sa53 poll --cmd '\{swrev?}'`

`--params` defaults to a conservative V1.1+ set that works on every firmware
version we ship; operators on V1.5+ firmware can pass extras explicitly
(e.g. `LaserTempSet,OscTuning,OvenCurrent,DCSignal`).

The tool refuses to run if anything else is holding the serial port, and
names the holder so you know what to stop.

## Layout

- `main.go` — 3-line shim → `cmd.Execute()`
- `cmd/` — cobra root + subcommands
- `protocol/` — SA53 serial protocol (was `mac/`)
- `detect/` — time card vendor detection
- `firmware/` — firmware file parser
- `xmodem/` — XModem 1K transfer
