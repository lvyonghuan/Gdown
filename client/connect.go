package client

import (
	"github.com/gorilla/websocket"
	"log"
	"net/http"
)

//与服务器对接

func connect() {
	wsURL := "ws://" + cfg.ServiceAdr + "/"
	header := http.Header{}
	header.Set("Authorization", cfg.token)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		log.Println("与服务器建立连接失败:", err)
		return
	}
	//心跳和断线检测
	heartBeat(conn)
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
