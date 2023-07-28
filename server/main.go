package main

import (
	"Gdown/server/src"
	"Gdown/server/src/user"
)

// 启动服务
func main() {
	user.InitDB()
	src.LoadFile()
	src.InitRouter()
}
