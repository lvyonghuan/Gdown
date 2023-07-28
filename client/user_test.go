package client

import (
	"sync"
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
	var wa sync.WaitGroup
	wa.Add(1)
	go initRouters()
	go downControl()
	downChan <- "Automation.mp3"
	wa.Wait()
}
