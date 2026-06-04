# caliper

caliper calculates the end-to-end optical latency from a GPS antenna to a GNSS
receiver.

It parses Luciol LOR-220 OTDR `.tor` files, auto-detects the reflective peaks
(OA, OB, OC, OD), computes the delays between them, generates SVG plots, and
writes the measurement data as a JSON file.

## Usage

```
caliper [flags] <tor-file>
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--serial` | _(required)_ | serial number of the Huber-Suhner GNSS receiver (format `PF` followed by 6 digits, e.g. `PF000142`) |
| `--name` | _(required)_ | device name (alphanumeric, dots, hyphens, underscores) |
| `--model` | `GNSSoF16-RxE` | receiver model: `GNSSoF16-RxE` or `GNSSPoF16-4RxE` |
| `--antenna-gen` | `gen2-p2` | antenna generation: `gen2-p0`, `gen2-p1`, `gen2-p2`, `gen2a-p2` |
| `--coax-cable-length` | `0` | length in meters of the RG58 coax cable at the SMA port |
| `--launch-cable-length` | `3.0` | length in meters of the launch cable (peaks within this distance are ignored) |
| `--output-dir`, `-o` | `.` | directory to write output files |
| `--loglevel` | `info` | log level: `debug`, `info`, `warning`, `error` |

## Output

For a device named `<name>`, caliper writes into `--output-dir`:

- `caliper_<name>.json` — measurement data
- `caliper_<name>.svg` — full OTDR trace plot
- `caliper_<name>_50ns.svg` — zoomed (50 ns) plot
- `caliper_<name>_cable_end.svg` — cable-end plot (when applicable)

## Example

```
caliper --serial PF000142 --name antenna01 \
        --model GNSSoF16-RxE --antenna-gen gen2-p2 \
        --coax-cable-length 1.5 -o ./results \
        measurement.tor
```
