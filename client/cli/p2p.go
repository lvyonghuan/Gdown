package cli

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

//p2p控制器，协调资源，作为客户端请求的入口和p2p相关问题的判断

type isDowning struct {
	mu        sync.Mutex
	filePiece map[int]string //key为起始位置，value为第几片的索引
}

var (
	isDowningQueue map[string]*isDowning //正在下载的文件队列
	hasDownedQueue map[string]struct{}   //已经下载的文件队列
)

func InitRouters() {
	r := gin.Default()
	r.GET("/down", getPiece)
	r.Run(":" + strconv.Itoa(cfg.ClientPort))
}

// 检查分片是否存在，获取分片，返回分片，处理错误请求
// 分片的存在得分情况：1，客户端正在下载被请求的文件（细分，已经下载了被请求的分片or没有下载）；2，客户端已
// 经下完了被请求的文件；3，客户端下完了被请求的文件，但是文件已经被删除or移动了。
// 4，客户端没有被请求的文件。
func getPiece(c *gin.Context) {
	//获取客户端请求的文件名
	var request struct {
		FileName string `json:"file_name"`
	}
	err := c.BindJSON(&request)
	if err != nil {
		c.JSON(400, gin.H{
			"message": "请求格式错误",
		})
		return
	}
	fileName := request.FileName

	fileRange := c.GetHeader("Range") //获取客户端请求的文件片段
	//解析文件的range，获取开头
	start, ok := getPieceStart(fileRange)
	if !ok {
		c.JSON(400, gin.H{
			"message": "range格式错误",
		})
		return
	}

	fileSize := c.GetHeader("Size")
	size, err := strconv.Atoi(fileSize)
	if err != nil {
		c.JSON(400, gin.H{
			"message": "Size格式错误",
		})
		return
	}

	//检查文件名是否存在各个文件列表中。
	fileData, ok := isDowningQueue[fileName]
	if ok {
		filePiece, isExist := getIsDowningFilePiece(start, fileData)
		if !isExist {
			c.JSON(400, gin.H{
				"message": "分片不存在",
			})
			return
		}
		c.Data(200, "application/octet-stream", filePiece)
		return
	}
	_, ok = hasDownedQueue[fileName]
	if ok {
		filePiece, isExist := getHasDownedFilePiece(start, size, fileName)
		if !isExist {
			c.JSON(400, gin.H{
				"message": "文件已移除",
			})
			return
		}
		c.Data(200, "application/octet-stream", filePiece)
		return
	}
	c.JSON(404, gin.H{
		"message": "文件不存在",
	})
}

func getIsDowningFilePiece(start int, fileData *isDowning) ([]byte, bool) {
	fileData.mu.Lock()
	defer fileData.mu.Unlock()
	//检查start是否在map当中
	fileName, ok := fileData.filePiece[start]
	if !ok {
		return nil, false
	}

	//读取临时文件
	file, err := os.ReadFile("./temp/" + fileName + ".tmp")
	if err != nil {
		return nil, false
	}

	return file, true
}

func getHasDownedFilePiece(start, size int, fileName string) ([]byte, bool) {
	file, err := os.Open("./down/" + fileName)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	filePiece := make([]byte, size)
	_, err = file.ReadAt(filePiece, int64(start))
	if err != nil {
		return nil, false
	}
	return filePiece, true
}

// 解析range
// 返回range的开头，另返回一个布尔值，代表是否解析成功
func getPieceStart(fileRange string) (int, bool) {
	// 确保 range 头不为空
	if fileRange == "" {
		return 0, false
	}

	// 判断 range 头是否以 "bytes=" 开头
	const prefix = "bytes="
	if !strings.HasPrefix(fileRange, prefix) {
		return 0, false
	}

	// 提取 range 头的值
	rangeValue := strings.TrimPrefix(fileRange, prefix)

	// 如果 range 头以 '-' 开头，表示不指定开头位置
	if rangeValue[0] == '-' {
		return 0, true
	}

	rangeParts := strings.Split(rangeValue, "-")

	// 解析 range 头的值为整数
	start, err := strconv.Atoi(rangeParts[0])
	if err != nil {
		return 0, false
	}

	return start, true
}

// 将已下载的文件列表传输到服务器上，同时初始化hasDownQueue
func sendFileList() {
	//初始化已下载文件队列
	hasDownedQueue = make(map[string]struct{})

	//获取已下载文件列表
	type fileList struct {
		FileName []string `json:"file_name"`
	}
	var fl fileList
	files, err := os.ReadDir("./down")
	if err != nil {
		log.Fatalf("读取已下载文件列表失败：%v", err)
	}

	for _, file := range files {
		fl.FileName = append(fl.FileName, file.Name())
		hasDownedQueue[file.Name()] = struct{}{}
	}

	//将已下载文件列表传输到服务器上
	u := "http://" + cfg.ServiceAdr + "/list"
	encodeData, err := json.Marshal(fl)
	if err != nil {
		log.Fatalf("序列化文件列表失败:%v", err)
	}

	req, err := http.NewRequest("POST", u, bytes.NewBuffer(encodeData))
	if err != nil {
		log.Fatalf("创建请求失败:%v", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", strconv.Itoa(len(encodeData)))
	req.Header.Set("User-Agent", "GDown")
	req.Header.Set("X-User-Port", strconv.Itoa(cfg.ClientPort))

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("发送请求失败:%v", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("读取服务器回传信息失败:%v", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("服务器回传信息错误:%v", string(body))
	}

	//获取真实ip
	type message struct {
		Message string `json:"message"`
		IpAdr   string `json:"ipAdr"`
	}
	var m message
	err = json.Unmarshal(body, &m)
	if err != nil {
		log.Fatalf("获取真实ip失败:%v", err)
	}
	trueIpAdr = m.IpAdr
}
