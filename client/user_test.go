package main_test

import (
	"Gdown/client/cli"
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
	cli.ReadConfig()
	cli.Login()
	var wa sync.WaitGroup
	wa.Add(1)
	go cli.InitRouters()
	go cli.DownControl()
	cli.DownChan <- "Automation.mp3"
	wa.Wait()
}
