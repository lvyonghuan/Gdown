package client

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
)

//下载模块
//1.获取元数据和拥有该文件的客户端ip列表。下载器将维护一个客户端ip池。
//2.根据元数据的分块，对服务器和客户端进行轮询操作。轮流请求文件的分片。
//3.假如对应客户端没有该文件的分片（对应客户端可能也在进行下载），则询问下一个客户端or服务器有无该文件分片。
//4.假如询问的客户端超过三次没有回应，则把它踢出维护的客户端ip池。
//5.每隔一段时间向文件服务器询问最新的客户端列表。
//6.对应3：如果一个客户端跑满了（达到了上传速率限制），则也询问下一个客户端or服务器。

var downChan = make(chan string, 10) //下载任务队列（限制同时下载的文件数）

// 下载引擎
type downEngine struct {
	fileName string   //下载的文件名
	ipAdr    []string //拥有该文件的客户端ip列表
	fileInfo fileInfo
	fileDate []byte
}

// 文件元数据
type fileInfo struct {
	FileName      string
	FilePiecesNum int
	FileSize      int
	FilePieces    []*piece
}

type piece struct {
	PieceIndex int    //第几片
	PieceStart int    //记录分片处在文件的起始位置
	PieceSize  int    //分片的大小
	PieceHash  uint32 //分片的哈希值，用于校验
}

// 分片数据，保证分片的排序一致性
type fileData struct {
	index int    //文件的实际分片数
	data  []byte //文件的数据
}

// 下载总控器
func downControl() {
	//初始化下载队列
	isDowningQueue = make(map[string]*isDowning)

	//启动下载进程
	for {
		select {
		case fileName := <-downChan:
			go fileHandler(fileName)
		}
	}
}

// 单个文件的下载控制调度器
func fileHandler(fileName string) {
	engine := newDownTask(fileName)                     //新建下载任务
	engine.getMetaData()                                //获取文件元数据
	pieceNum := engine.fileInfo.FilePiecesNum           //获取文件的分片数
	engine.ipAdr = append(engine.ipAdr, cfg.ServiceAdr) //将服务器也作为一个下载节点

	var fileQueue []fileData                                                  //文件队列，用于记录下载成功的分片，按照顺序进行排列
	var failQueue []int                                                       //失败队列，用于记录下载失败的分片的索引
	isDowningQueue[fileName] = &isDowning{filePiece: make(map[int]*fileData)} //将文件加入到正在下载的队列中

	successNum := 0 //下载成功的分片数

	for i, j := 0, 0; i < pieceNum; i++ { //轮询多线程下载
		var client string
		//获取除了自己以外的client
		for j++; ; j++ {
			if j >= len(engine.ipAdr) {
				j = 0
			}
			if engine.ipAdr[j] != "127.0.0.1:"+strconv.Itoa(cfg.ClientPort) {
				client = engine.ipAdr[j]
				break
			}
		}

		//下载分片
		data, isSuccess := engine.downPiece(i, client)
		if !isSuccess {
			failQueue = append(failQueue, i)
			continue
		}

		//将分片加入到文件队列中
		fileQueue = append(fileQueue, fileData{i, data})

		//打印下载进度
		successNum++
		percentage := float64(successNum) / float64(pieceNum) * 100
		log.Printf(fileName+"下载进度：%.2f%%", percentage)

		//将分片加入到正在下载队列中。使用goroutine，避免阻塞主线程。
		k := i
		go func() {
			isDowningQueue[fileName].mu.Lock()
			isDowningQueue[fileName].filePiece[engine.fileInfo.FilePieces[k].PieceStart] = &fileData{k, data}
			isDowningQueue[fileName].mu.Unlock()
		}()
	}

	//重新下载失败的队列
	for _, i := range failQueue {
		data, isSuccess := engine.downPiece(i, cfg.ServiceAdr) //直接从服务器获取失败的数据
		if !isSuccess {
			log.Println("第" + strconv.Itoa(i) + "片下载失败,文件损坏，请重新下载") //再失败就不重试了
			//TODO:其实也可以再重试几轮
		}
		fileQueue = append(fileQueue, fileData{i, data})
		successNum++
		percentage := float64(successNum) / float64(pieceNum) * 100
		log.Printf(fileName+"下载进度：%.2f%%", percentage)

		k := i
		go func() {
			isDowningQueue[fileName].mu.Lock()
			isDowningQueue[fileName].filePiece[engine.fileInfo.FilePieces[k].PieceStart] = &fileData{k, data}
			isDowningQueue[fileName].mu.Unlock()
		}()
	}

	//将文件队列按照顺序写入文件
	writeFile(fileQueue, fileName)
	//将文件从正在下载队列中移除
	isDowningQueue[fileName].mu.Lock()
	delete(isDowningQueue, fileName)
	hasDownedQueue[fileName] = struct{}{} //将文件加入到已下载队列中
}

// 新建下载任务
func newDownTask(fileName string) *downEngine {
	var d downEngine
	d.fileName = fileName
	d.getMetaData()
	return &d
}

// 从服务器获取元数据，实际上就是获取种子文件
func (d *downEngine) getMetaData() {
	u := "http://" + cfg.ServiceAdr + "/meta"

	var data struct {
		FileName string `json:"file_name"`
	}
	data.FileName = d.fileName
	encodeData, err := json.Marshal(data)
	if err != nil {
		log.Println("序列化文件名失败:", err)
		return
	}

	req, err := http.NewRequest("GET", u, bytes.NewBuffer(encodeData))
	if err != nil {
		log.Println("创建请求失败:", err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", strconv.Itoa(len(encodeData)))
	req.Header.Set("User-Agent", "GDown")
	req.Header.Set("X-User-Port", strconv.Itoa(cfg.ClientPort))

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("发送请求失败:", err)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("读取服务器回传信息失败:", err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Println(resp.StatusCode, ":", string(body))
		return
	}
	//反序列化元数据
	var meta struct {
		Message []byte   `json:"message"`
		IpAdr   []string `json:"ip_adr"`
	}
	err = json.Unmarshal(body, &meta)
	if err != nil {
		log.Println("解析服务器回传信息失败:", err)
		return
	}
	//将元数据写入到文件中
	err = os.WriteFile("./fileInfo/"+d.fileName+".god", meta.Message, 0666)
	d.ipAdr = meta.IpAdr
	d.unmarshalGod()
}

// 解析元数据
func (d *downEngine) unmarshalGod() {
	file, err := os.Open("./fileInfo/" + d.fileName + ".god")
	if err != nil {
		log.Println("打开元数据文件失败:", err)
		return
	}
	defer file.Close()
	var meta fileInfo
	dec := gob.NewDecoder(file)
	if err := dec.Decode(&meta); err != nil {
		log.Println("解析元数据失败:", err)
		return
	}
	d.fileInfo = meta
}

// 下载分片数据
func (d *downEngine) downPiece(index int, client string) ([]byte, bool) {
	downLimitGet()   //下载限速，获取令牌
	defer downDown() //放回令牌
	u := "http://" + client + "/down"
	var data struct {
		FileName string `json:"file_name"`
	}
	data.FileName = d.fileName
	encodeData, err := json.Marshal(data)
	if err != nil {
		log.Println("序列化文件名失败:", err)
		return nil, false
	}

	req, err := http.NewRequest("GET", u, bytes.NewBuffer(encodeData))
	if err != nil {
		log.Println("创建请求失败:", err)
		return nil, false
	}

	start := d.fileInfo.FilePieces[index].PieceStart
	end := start + d.fileInfo.FilePieces[index].PieceSize
	startStr := strconv.Itoa(start)
	endStr := strconv.Itoa(end)

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", strconv.Itoa(len(encodeData)))
	req.Header.Set("User-Agent", "GDown")
	req.Header.Set("Range", "bytes="+startStr+"-"+endStr)
	req.Header.Set("Size", strconv.Itoa(d.fileInfo.FilePieces[index].PieceSize))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("发送请求失败:", err)
		return nil, false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("读取服务器回传信息失败:", err)
		return nil, false
	}
	if resp.StatusCode != http.StatusOK {
		log.Println(resp.StatusCode, ":", string(body))
		return nil, false
	}
	if hash(body) != d.fileInfo.FilePieces[index].PieceHash {
		log.Println("第" + strconv.Itoa(index) + "片校验失败")
		return nil, false
	}
	return body, true
}

// 写入文件
func writeFile(filesData []fileData, fileName string) {
	//将队列按照顺序进行排序，保证一致性
	sort.Slice(filesData, func(i, j int) bool {
		return filesData[i].index < filesData[j].index
	})

	//写文件
	file, err := os.Create("./down/" + fileName)
	if err != nil {
		log.Println("创建文件失败:", err)
		return
	}
	defer file.Close()
	for _, data := range filesData {
		_, err = file.Write(data.data)
		if err != nil {
			log.Println("写入文件失败:", err)
			return
		}
	}
}

// 哈希值校验
func hash(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
