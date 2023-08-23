# Calnex
Command line tool for a Calnex Sentinel device
Cli Supports several basic commands such as:
* Firmware upgrade
* Configuration of the device
* Measurement data export
* Device reboot
* Device clear
* Device problem report export
* Device verify

```
$ calnex firmware --target calnex01.example.com --file ~/go/github.com/facebook/time/calnex/testdata/sentinel_fw_v3.0.tar
INFO[0000] calnex01.example.com is running 2.1, latest is 3.0.0. Needs an update
INFO[0000] dry run. Exiting
```
