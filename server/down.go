package server

import (
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// 向客户端发送文件元数据
func sendMetaDate(c *gin.Context) {
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

	//检查文件名是否存在在文件列表中
	_, ok := fileLists[fileName]
	if !ok {
		c.JSON(404, gin.H{
			"message": "文件不存在",
		})
		return
	}
	//获取文件的元数据文件
	file, err := os.ReadFile("./fileInfo/" + fileName + ".god")
	if err != nil {
		c.JSON(500, gin.H{
			"message": "服务器内部错误",
		})
		return
	}
	//将此客户端的ip地址加入到文件的ip列表当中去
	ip := c.ClientIP() + ":" + c.GetHeader("X-User-Port")
	fileLists[fileName].ipAdr.Store(ip, true)
	//获取拥有此文件的客户端的IP地址
	var ipAdr []string
	fileLists[fileName].ipAdr.Range(func(key, value interface{}) bool {
		ipAdr = append(ipAdr, key.(string))
		return true
	})
	//发送文件的元数据
	c.JSON(200, gin.H{
		"message": file,
		"ip_adr":  ipAdr,
	})
}

// 向客户端发送请求的文件数据片段
func sendFilePiece(c *gin.Context) {
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
	//检查文件名是否存在在文件列表中
	fileInformation, ok := fileLists[fileName]
	if !ok {
		c.JSON(404, gin.H{
			"message": "文件不存在",
		})
		return
	}
	//解析文件的range，获取开头
	start, ok := getPieceStart(fileRange)
	if !ok {
		c.JSON(400, gin.H{
			"message": "range格式错误",
		})
		return
	}
	//检查start是否在map当中
	p, ok := fileInformation.filePiecesByStart[start]
	if !ok {
		c.JSON(400, gin.H{
			"message": "range格式错误",
		})
		return
	}
	//获取文件的片段
	filePiece := getFilePiece(fileName, start, p.PieceSize)
	//发送文件的片段
	c.Data(200, "application/octet-stream", filePiece)
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

// 获取指定文件的指定片段
func getFilePiece(fileName string, pieceStart, pSize int) []byte {
	file, err := os.Open("./file/" + fileName)
	if err != nil {
		return nil
	}
	defer file.Close()
	filePiece := make([]byte, pSize)
	_, err = file.ReadAt(filePiece, int64(pieceStart))
	if err != nil {
		return nil
	}
	return filePiece
}
