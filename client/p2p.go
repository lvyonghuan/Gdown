package client

import (
	"github.com/gin-gonic/gin"
	"os"
	"strconv"
	"strings"
	"sync"
)

//p2p控制器，协调资源，作为客户端请求的入口和p2p相关问题的判断

type isDowning struct {
	mu        sync.Mutex
	filePiece map[int]*fileData //正在下载的文件队列。key为分片的起始位置，value为分片的数据
}

var (
	isDowningQueue map[string]*isDowning //正在下载的文件队列
	hasDownedQueue map[string]struct{}   //已经下载的文件队列
)

func initRouters() {
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
		filePiece, isExist := getHasDownedFilePiece(start, fileName)
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
	p, ok := fileData.filePiece[start]
	if !ok {
		return nil, false
	}
	return p.data, true
}

func getHasDownedFilePiece(start int, fileName string) ([]byte, bool) {
	file, err := os.Open("./down/" + fileName)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	filePiece := make([]byte, blockSize) //这大概会对最后一片有影响？
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