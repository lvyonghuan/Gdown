package client

import (
	"testing"
)

func TestRegister(t *testing.T) {
	readConfig()
	register()
}

func TestLogin(t *testing.T) {
	readConfig()
	login()
}
