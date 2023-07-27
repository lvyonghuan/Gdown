package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"Gdown/server/user"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type client struct {
	IPAdr        string               //记录客户端的IP地址
	DownFileList map[string]*FileInfo //维护客户端下载的文件列表。客户端下线的时候，从每个文件列表里把它删掉。
}

var clientList = make(map[string]bool) //维护客户端列表。

var upgrade = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true //允许跨域
	},
}

//服务器与客户端建立联系，提供下载
//使用gin框架建立连接，再采用gorilla/websocket进行长链接。使用心跳检测检测各个客户端是否在线。

// InitRouter 初始化路由
func InitRouter() {
	r := gin.Default()
	u := r.Group("/user")
	{
		u.GET("/login", user.Login)
		u.POST("/register", user.Register)
	}
	r.GET("/", connect)           //客户端与服务器建立连接。
	r.POST("/list", getFileList)  //客户端向服务器发送已经下载的文件的列表
	r.GET("/meta", sendMetaDate)  //客户端下载文件，服务器返回此文件的元数据和拥有此文件的客户端的IP地址
	r.GET("/down", sendFilePiece) //下载具体的分片
	r.Run()
}

func getFileList(c *gin.Context) {
	if isConnect, ok := clientList[c.ClientIP()]; !ok || !isConnect {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "客户端未建立连接",
		})
		return
	}
	type fileList struct {
		FileName []string `json:"file_name"`
	}
	var fl fileList
	err := c.BindJSON(&fl)
	if err != nil {
		log.Println("客户端发送的文件列表格式错误：", err)
		return
	}
	for _, fileName := range fl.FileName {
		if _, ok := fileLists[fileName]; !ok { //健壮性检查
			continue
		}
		fileLists[fileName].ipAdr.Store(c.ClientIP()+getPort(c.Request), true) //将客户端ip追加到文件的列表当中去
	}
}

// websocket连接的处理函数,用于检测客户端是否下线
// connect 客户端与服务器建立websocket长连接
// 进行客户端鉴权
func connect(c *gin.Context) {
	//验证token
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "未授权",
		})
		return
	}
	claims, err := user.ParseToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "未授权",
		})
		return
	}
	if !claims {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "未授权",
		})
		return
	}

	var cli client
	cli.IPAdr = c.ClientIP() + ":" + c.GetHeader("X-User-Port")
	clientList[cli.IPAdr] = true
	conn, err := upgrade.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("与客户端建立连接失败，websocket升级错误：", err)
		return
	}
	heartBeat(conn) //心跳和断线检测，并不需要并发读写操作，直接阻塞就行了
	//当心跳断开的时候
	deleteFileIP(&cli)
	delete(clientList, cli.IPAdr) //从客户端列表里删掉这个客户端
}

// 心跳和断线检测
func heartBeat(conn *websocket.Conn) {
	var ticker = time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	defer conn.Close()
	var stopChan = make(chan bool) //用于停止进程

	//发送心跳
	go func() {
		for {
			select {
			case <-ticker.C:
				err := conn.WriteMessage(websocket.TextMessage, []byte("ping!"))
				if err != nil {
					if err.Error() == "websocket: close sent" {
						return
					}
					log.Println("发送心跳失败：", err)
					stopChan <- true
					return
				}
			}
		}
	}()

	//检测客户端断线
	go func() {
		for {
			err := conn.SetReadDeadline(time.Now().Add(125 * time.Second)) //重置读取截止时间
			if err != nil {
				log.Println("设置读取截止时间失败：", err)
				stopChan <- true
				return
			}
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println("客户端断线：", err)
				stopChan <- true
				return
			}
			log.Println(msg)
		}
	}()

	//停止进程
	for {
		select {
		case <-stopChan:
			return
		}
	}
}

// 当客户端断线的时候，删除文件信息里客户端的ip地址
func deleteFileIP(cli *client) {
	for i := range cli.DownFileList {
		file := cli.DownFileList[i]
		file.ipAdr.Delete(cli.IPAdr)
	}
}

// 获取客户端端口号
func getPort(r *http.Request) string {
	ip := strings.Split(r.RemoteAddr, ":")
	log.Println(ip)
	return ":" + ip[1] //返回端口号
}
