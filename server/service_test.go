package server

import "testing"

func TestRouter(t *testing.T) {
	loadFile() //加载文件，以便测试
	InitRouter()
}
