package client

import (
	"testing"
)

//func TestRegister(t *testing.T) {
//	readConfig()
//	register()
//}
//
//func TestLogin(t *testing.T) {
//	readConfig()
//	login()
//	connect()
//}

func TestDown(t *testing.T) {
	readConfig()
	login()
	fileHandler("Automation.mp3")
}
