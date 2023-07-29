package cli

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

//下载模块
//1.获取元数据和拥有该文件的客户端ip列表。下载器将维护一个客户端ip池。
//2.根据元数据的分块，对服务器和客户端进行轮询操作。轮流请求文件的分片。
//3.假如对应客户端没有该文件的分片（对应客户端可能也在进行下载），则询问下一个客户端or服务器有无该文件分片。
//4.假如询问的客户端超过三次没有回应，则把它踢出维护的客户端ip池。
//5.每隔一段时间向文件服务器询问最新的客户端列表。
//6.对应3：如果一个客户端跑满了（达到了上传速率限制），则也询问下一个客户端or服务器。

var (
	DownChan        = make(chan string, 10)  //下载任务队列（限制同时下载的文件数）
	downMessageChan = make(chan downMessage) //下载消息队列，用于多线程下载
)

// 下载引擎
type downEngine struct {
	fileName   string   //下载的文件名
	ipAdr      []string //拥有该文件的客户端ip列表
	fileInfo   fileInfo
	fileQueue  []tempFileInfo //文件队列，用于记录下载成功的分片，按照顺序进行排列
	downQueue  []int          //下载队列，排队下载
	successNum int            //下载成功的分片数
	finish     chan string    //下载完成信号
	wg         sync.WaitGroup //等待所有分片下载完成
	mu         sync.Mutex     //防止并发写successNum的时候冲突
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

// 临时文件信息
type tempFileInfo struct {
	index int
	name  string
}

// 管道传递的消息
type downMessage struct {
	index  int
	client string
}

// DownControl 下载总控器
func DownControl() {
	//初始化下载队列
	isDowningQueue = make(map[string]*isDowning)

	//启动下载进程
	for {
		select {
		case fileName := <-DownChan:
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

	isDowningQueue[fileName] = &isDowning{filePiece: make(map[int]string)} //将文件加入到正在下载的队列中

	engine.downQueue = make([]int, 0, pieceNum)
	//初始化下载队列
	for i := 0; i < pieceNum; i++ {
		engine.downQueue = append(engine.downQueue, i)
	}

	go multithreadingControl(engine)

	var j = 0 //p2p服务端轮询控制
	for engine.successNum < pieceNum {
		for _, i := range engine.downQueue {
			var client string
			//获取除了自己以外的client
			for j++; ; j++ {
				if j >= len(engine.ipAdr) {
					j = 0
				}
				if engine.ipAdr[j] != trueIpAdr+":"+strconv.Itoa(cfg.ClientPort) {
					client = engine.ipAdr[j]
					break
				}
			}
			downMessageChan <- downMessage{i, client}
			engine.downQueue = engine.downQueue[1:] //移除遍历到的元素
		}
		time.Sleep(time.Second * 1)
	}

	engine.wg.Add(1)
	//下载完成，发送下载完成信号
	engine.finish <- fileName
	engine.wg.Wait()
}

// 多线程下载控制器
func multithreadingControl(engine *downEngine) {
	for {
		select {
		case msg := <-downMessageChan:
			go func() {
				downLimitGet()   //下载限速，获取令牌
				defer downDown() //放回令牌
				data, isSuccess := engine.downPiece(msg.index, msg.client)
				if !isSuccess {
					engine.downQueue = append(engine.downQueue, msg.index) //下载失败，重新加入到下载队列中
					return
				}
				if !writeTempFile(data, msg.index, engine.fileName) {
					engine.downQueue = append(engine.downQueue, msg.index) //下载失败，重新加入到下载队列中
					return
				}

				engine.mu.Lock() //防止并发写successNum的时候冲突
				//将分片加入到文件队列中
				engine.fileQueue = append(engine.fileQueue, tempFileInfo{msg.index, "./temp/" + engine.fileName + strconv.Itoa(msg.index) + ".tmp"})

				//打印下载进度
				engine.successNum++
				percentage := float64(engine.successNum) / float64(engine.fileInfo.FilePiecesNum) * 100
				log.Printf(engine.fileName+"下载进度：%.2f%%", percentage)
				engine.mu.Unlock()
			}()
		case fileName := <-engine.finish:
			writeFile(engine.fileQueue, fileName)
			//将文件从正在下载队列中移除
			isDowningQueue[fileName].mu.TryLock()
			delete(isDowningQueue, fileName)
			hasDownedQueue[fileName] = struct{}{} //将文件加入到已下载队列中
			engine.wg.Done()
			return
		}
	}
}

// 新建下载任务
func newDownTask(fileName string) *downEngine {
	var d downEngine
	d.fileName = fileName
	d.successNum = 0
	d.finish = make(chan string)
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

	client := http.Client{
		Timeout: time.Second * 30, //设置超时时间
	}
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", u, bytes.NewBuffer(encodeData))
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

	c := http.Client{}
	resp, err := c.Do(req)
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
func writeFile(filesData []tempFileInfo, fileName string) {
	//将队列按照顺序进行排序，保证一致性
	sort.Slice(filesData, func(i, j int) bool {
		return filesData[i].index < filesData[j].index
	})

	//合并临时文件
	file, err := os.OpenFile("./down/"+fileName, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println("创建文件失败:", err)
		return
	}
	defer file.Close()
	for _, fileData := range filesData {
		data, err := os.ReadFile(fileData.name)
		if err != nil {
			log.Println("读取临时文件失败:", err)
			return
		}
		_, err = file.Write(data)
		if err != nil {
			log.Println("写入文件失败:", err)
			return
		}
		filesData = filesData[1:] //移除遍历到的元素
		err = os.Remove(fileData.name)
		if err != nil {
			log.Println("删除临时文件失败:", err)
			continue
		}
	}
	log.Println(fileName + "下载完成")
}

// 写临时文件
func writeTempFile(data []byte, index int, fileName string) bool {
	tempFile, err := os.Create("./temp/" + fileName + strconv.Itoa(index) + ".tmp")
	if err != nil {
		return false
	}

	defer tempFile.Close()
	_, err = tempFile.Write(data)
	if err != nil {
		return false
	}
	return true
}

// 哈希值校验
func hash(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
