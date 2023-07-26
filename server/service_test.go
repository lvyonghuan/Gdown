package server

import (
	"Gdown/server/user"
	"testing"
)

func TestRouter(t *testing.T) {
	user.InitDB()
	loadFile() //加载文件，以便测试
	InitRouter()
}
