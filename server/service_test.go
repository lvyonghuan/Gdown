package main_test

import (
	"Gdown/server/src"
	"Gdown/server/src/user"
	"testing"
)

func TestRouter(t *testing.T) {
	user.InitDB()
	src.LoadFile() //加载文件，以便测试
	src.InitRouter()
}
