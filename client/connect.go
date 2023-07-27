package client

import (
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strconv"
)

//与服务器对接

// 与服务器建立websocket连接，并且进行持续性的心跳检测
func connect() {
	wsURL := "ws://" + cfg.ServiceAdr + "/"
	header := http.Header{}
	header.Set("Authorization", cfg.token)
	header.Set("X-User-Port", strconv.Itoa(cfg.ClientPort))

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		log.Println("与服务器建立连接失败:", err)
		return
	}
	//开启配置文件监视器
	go hotReset()
	//心跳和断线检测
	go heartBeat(conn)
	//TODO:断线重连
}

// 心跳和断线检测
func heartBeat(conn *websocket.Conn) {
	defer conn.Close()
	for {
		typ, _, err := conn.ReadMessage()
		if err != nil {
			log.Println("与服务器断开连接:", err)
			return
		}
		if typ == websocket.TextMessage {
			log.Println("ping!")
			err = conn.WriteMessage(websocket.TextMessage, []byte("pong!"))
			if err != nil {
				log.Println("发送心跳包失败:", err)
				return
			}
		}
	}
}
