package server

//处理文件信息，对服务器内文件进行“做种”处理

import (
	"bytes"
	"encoding/gob"
	"hash/crc32"
	"log"
	"os"
	"sync"
)

const pieceSize = 1024 * 1024 //每个分片的大小

var fileLists = make(map[string]*FileInfo) //维护文件列表

type FileInfo struct {
	FileName          string
	FilePiecesNum     int
	FileSize          int
	FilePieces        []*Piece
	ipAdr             sync.Map       //记录下载此文件的客户端的IP地址。用哈希表是因为查询时间复杂度低，而且这里要保证并发安全性，防止客户端下线的时候读写冲突。
	filePiecesByStart map[int]*Piece //记录分片的起始位置，用于客户端请求分片的时候进行快速查找
}

type Piece struct {
	PieceIndex int    //第几片
	PieceStart int    //记录分片处在文件的起始位置
	PieceSize  int    //分片的大小
	PieceHash  uint32 //分片的哈希值，用于校验
}

// 遍历file目录，加载file目录下的文件，并且将它们分片存储，最后格式化为toml配置文件
func loadFile() {
	files, err := os.ReadDir("./file")
	if err != nil {
		log.Fatalf(err.Error())
	}
	for _, file := range files {
		if file.Name() == "README.md" {
			continue //跳过readme，这东西放文件夹里做提示用的
		}
		handelFile(file)
	}
}

// 处理文件
func handelFile(f os.DirEntry) {
	var info FileInfo
	info.ipAdr = sync.Map{}
	info.FileName = f.Name()
	info.filePiecesByStart = make(map[int]*Piece)
	fileInfo, err := f.Info()
	if err != nil {
		log.Println(info.FileName, "处理错误，获取info失败：", err)
		return //如果错误直接返回就是
	}

	info.FileSize = int(fileInfo.Size())                        //获取文件的大小
	info.FilePiecesNum = chunkFileNum(info.FileSize, pieceSize) //确定文件该分成多少片
	err = chunkFile(&info, pieceSize)                           //进行分片

	if err != nil {
		return
	}
	//将文件信息格式化为文件。
	//TODO:参考https://stackoverflow.com/questions/65842245/what-does-the-error-binary-write-invalid-type-mean,之前尝试用二进制文件存储出错了，查了知道binary包的那个不支持动态的数据写入。
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(info); err != nil {
		log.Println(info.FileName, "处理错误，编码失败：", err)
		return
	}
	fileLists[info.FileName] = &info //将文件加入文件列表当中
	//写文件
	//将文件信息编码进一个二进制文件当中，客户端发起下载请求，服务器将文件发送给各个客户端。客户端对二进制文件进行反序列化，获得该文件的分片信息。再向服务器进行询问
	//服务器记录客户端的IP地址，将文件分片进行发送（客户端发送片段请求，服务器发送片段，客户端组合片段），并将客户端的IP地址记录在文件信息中
	file, err := os.Create("./fileInfo/" + info.FileName + ".god")
	if err != nil {
		log.Println(info.FileName, "处理错误，创建文件失败：", err)
		return
	}
	defer file.Close()
	_, err = file.Write(buf.Bytes())
	if err != nil {
		log.Fatal("处理错误，写入文件失败：", err)
		return
	}
}

// 确定文件分片的数目
func chunkFileNum(fileSize int, pieceSize int) int {
	chunks := fileSize / pieceSize
	if fileSize%pieceSize != 0 {
		chunks++
	}
	return chunks
}

// 进行分片操作
func chunkFile(file *FileInfo, pieceSize int) error {
	file.FilePieces = make([]*Piece, file.FilePiecesNum)
	f, err := os.Open("./file/" + file.FileName) //加载文件的具体数据
	if err != nil {
		log.Println(file.FileName, "分片错误，加载文件失败：", err)
		return err
	}

	defer f.Close()
	for i := 0; i < file.FilePiecesNum; i++ {
		var p Piece

		p.PieceIndex = i
		p.PieceStart = i * pieceSize //记录分片处在文件的起始位置
		if i != 0 {
			p.PieceStart += 1
		}

		if i == file.FilePiecesNum-1 {
			p.PieceSize = file.FileSize - i*pieceSize - 1
		} else {
			p.PieceSize = pieceSize
		}

		pieceData := make([]byte, p.PieceSize)
		_, err := f.ReadAt(pieceData, int64(p.PieceStart))
		if err != nil {
			log.Println(file.FileName, "处理错误，文件分片失败：", err)
			return err
		}
		p.PieceHash = hash(pieceData)
		file.FilePieces[i] = &p
		file.filePiecesByStart[p.PieceStart] = &p
	}
	return nil
}

// 计算哈希值
func hash(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
