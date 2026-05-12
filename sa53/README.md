# sa53

Tools for the Microchip SA53 atomic clock on Celestica time cards.

Built as a single binary `sa53` (installed to `/usr/local/bin/sa53fw` via the
`fb-sa53fw` RPM, name kept for chef compatibility) using cobra subcommands.

## Subcommands

### firmware

Read or upgrade SA53 firmware.

```
sudo sa53fw firmware --check                          # print version as JSON
sudo sa53fw firmware --fw firmware.hex --upgrade      # apply an upgrade
sudo sa53fw firmware --fw firmware.hex --upgrade --force
```

Old `sa53fw -upgrade` style is gone; cobra requires the subcommand and `--`
long flags.

## Layout

- `main.go` — 3-line shim → `cmd.Execute()`
- `cmd/` — cobra root + subcommands
- `protocol/` — SA53 serial protocol (was `mac/`)
- `detect/` — time card vendor detection
- `firmware/` — firmware file parser
- `xmodem/` — XModem 1K transfer
