package control

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNormalizeData(t *testing.T) {
	assert := assert.New(t)
	data := []byte(
		`version="ntpd 4.2.6p5@1.2349-o Fri Apr 13 12:52:27 UTC 2018 (1)",
processor="x86_64", system="Linux/4.11.3-61_fbk16_3934_gd064a3c",
leap=0, stratum=4, precision=-24, rootdelay=64.685, rootdisp=76.350,
refid=174.141.68.116, reftime=0xdfb39d2d.8598591b,
clock=0xdfb39fbe.dd542f86, peer=60909, tc=10, mintc=3, offset=-0.180,
frequency=0.314, sys_jitter=0.246, clk_jitter=0.140, clk_wander=0.009
`)
	parsed, err := NormalizeData(data)

	assert.Nil(err)
	expected := map[string]string{
		"version":   "ntpd 4.2.6p5@1.2349-o Fri Apr 13 12:52:27 UTC 2018 (1)",
		"processor": "x86_64", "system": "Linux/4.11.3-61_fbk16_3934_gd064a3c",
		"leap":       "0",
		"stratum":    "4",
		"precision":  "-24",
		"rootdelay":  "64.685",
		"rootdisp":   "76.350",
		"refid":      "174.141.68.116",
		"reftime":    "0xdfb39d2d.8598591b",
		"clock":      "0xdfb39fbe.dd542f86",
		"peer":       "60909",
		"tc":         "10",
		"mintc":      "3",
		"offset":     "-0.180",
		"frequency":  "0.314",
		"sys_jitter": "0.246",
		"clk_jitter": "0.140",
		"clk_wander": "0.009",
	}
	assert.Equal(expected, parsed)
}

// test that we skip bad pairs
func TestNormalizeDataCorrupted(t *testing.T) {
	assert := assert.New(t)
	data := []byte(`srcadr=2401:db00:3110:5068:face:0:5c:0, srcport=123,
dstadr=2401:db00:3110:915d:face:0:5a:0, dstport=123, leap=0, stratum=3,
precision=-24, rootdelay=83.313, rootdisp=47.607, refid=1.104.123.73,
reftime=0xdfb8e24c.b57496e4, rec=0xdfb8e395.93319ff3, reach=0xff,
unreach=0, hmode=3, pmode=4, hpoll=7, ppoll=7, headway=8, flash=0x0,
keyid=0, offset=0.163, delay=0.136, dispersion=5.123, jitter=0.054,
xleave=0.022, filtdelay= 0.33 0.16 0.14 0.27 0.27 0.29 0.18 0.24filtoffset= 0.17 0.19 0.16 0.12 0.09 0.11 0.09 0.10,
filtdisp= 0.00 1.95 3.87 5.79 7.79 9.78 11.72 13.71
`)
	parsed, err := NormalizeData(data)

	assert.Nil(err)
	expected := map[string]string{
		"delay":      "0.136",
		"dispersion": "5.123",
		"dstadr":     "2401:db00:3110:915d:face:0:5a:0",
		"dstport":    "123",
		"filtdisp":   "0.00 1.95 3.87 5.79 7.79 9.78 11.72 13.71",
		"flash":      "0x0",
		"headway":    "8",
		"hmode":      "3",
		"hpoll":      "7",
		"jitter":     "0.054",
		"keyid":      "0",
		"leap":       "0",
		"offset":     "0.163",
		"pmode":      "4",
		"ppoll":      "7",
		"precision":  "-24",
		"refid":      "1.104.123.73",
		"reftime":    "0xdfb8e24c.b57496e4",
		"rootdelay":  "83.313",
		"rootdisp":   "47.607",
		"stratum":    "3",
		"reach":      "0xff",
		"rec":        "0xdfb8e395.93319ff3",
		"srcadr":     "2401:db00:3110:5068:face:0:5c:0",
		"srcport":    "123",
		"unreach":    "0",
		"xleave":     "0.022",
	}
	assert.Equal(expected, parsed)
}
