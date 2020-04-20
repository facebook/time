package server

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ConfigSet(t *testing.T) {
	testIP := "1.2.3.4"

	m := MultiIPs{}
	m.Set(testIP)

	assert.Equal(t, net.ParseIP(testIP), m[0])
}

func Test_ConfigSetInvalid(t *testing.T) {
	testIP := "invalid"

	m := MultiIPs{}
	m.Set(testIP)

	assert.Empty(t, m)
}

func Test_ConfigString(t *testing.T) {
	testIP1 := "1.2.3.4"
	testIP2 := "5.6.7.8"

	m := MultiIPs{}
	m.Set(testIP1)
	m.Set(testIP2)

	assert.Equal(t, m.String(), fmt.Sprintf("%s, %s", testIP1, testIP2))
}

func Test_ConfigSetDefault(t *testing.T) {
	m := MultiIPs{}
	m.SetDefault()

	assert.Equal(t, DefaultServerIPs, m)
}
