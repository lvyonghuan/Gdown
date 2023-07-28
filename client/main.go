package main

import (
	"Gdown/client/cli"
	"fmt"
)

// 客户端启动
func main() {
	const (
		register = 1
		login    = 2
		download = 3
		exit     = 4
	)
	for {
		fmt.Println("1.注册\n2.登录\n3.下载\n4.退出")
		var choice int
		_, err := fmt.Scanln(&choice)
		if err != nil {
			fmt.Println("错误输入")
			continue
		}
		switch choice {
		case register:
			cli.ReadConfig()
			cli.Register()
		case login:
			cli.ReadConfig()
			cli.Login()
			go cli.InitRouters()
			go cli.DownControl()
		case download:
			var filename string
			fmt.Println("请输入要下载的文件名:")
			_, err := fmt.Scanln(&filename)
			if err != nil {
				fmt.Println("错误输入")
				continue
			}
			cli.DownChan <- filename
		case exit:
			return
		default:
			fmt.Println("错误的输入")
		}
	}
}
