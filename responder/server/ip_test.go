package server

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testIP = "1.2.3.4"

func Test_checkIP(t *testing.T) {
	iface, err := net.InterfaceByName("lo")
	assert.Nil(t, err)

	ip := net.ParseIP("::1")
	assigned, err := checkIP(iface, &ip)

	assert.Nil(t, err)
	assert.True(t, assigned)
}

func Test_checkIPFalse(t *testing.T) {
	iface, err := net.InterfaceByName("lo")
	assert.Nil(t, err)

	ip := net.ParseIP("8.8.8.8")
	assigned, err := checkIP(iface, &ip)

	assert.Nil(t, err)
	assert.False(t, assigned)
}

func Test_addIPToInterfaceError(t *testing.T) {
	lc := ListenConfig{Iface: "lol-does-not-exist"}
	s := &Server{ListenConfig: lc}
	err := s.addIPToInterface(net.ParseIP(testIP))
	assert.NotNil(t, err)
}

func Test_deleteIPFromInterfaceError(t *testing.T) {
	lc := ListenConfig{Iface: "lol-does-not-exist"}
	s := &Server{ListenConfig: lc}
	err := s.deleteIPFromInterface(net.ParseIP(testIP))
	assert.NotNil(t, err)
}
