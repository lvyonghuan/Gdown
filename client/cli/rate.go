package cli

import (
	"sync"
)

//限速器。采用令牌桶算法，读取配置文件，实现下载速度和上传速度的限制。
//简单来说，配置文件配置速度/每块分片的大小，获得可以同时进行传输的分片数量，从而达到速率限制的效果

// 上传速度限制器
type upL struct {
	upLimit int        //上传速度最大限制
	upCount int        //已经在上传的分片数量
	mu      sync.Mutex //并发安全
}

var (
	downLimit chan struct{}
	upLimit   upL
) //下载速度限制

const blockSize = 1024 * 1024 //每个分片的大小。服务器制造固定大小的分片，同步。

// 限速器。读取配置文件，构造令牌桶。
func limit() {
	downRateLimit := cfg.DownRate
	upRateLimit := cfg.UpRate

	//构造下载限速令牌桶
	if downRateLimit != 0 {
		downLimit = make(chan struct{}, downRateLimit/blockSize)
	} else {
		downLimit = nil
	}

	//构造上传限速令牌桶
	if upRateLimit != 0 {
		upLimit.upLimit = upRateLimit / blockSize
	} else {
		upLimit.upLimit = 0
	}
}

// 下载限速器。从令牌桶中取出一个令牌，如果没有令牌则阻塞。
func downLimitGet() {
	if downLimit != nil { //如果没有限速，则不启用令牌桶。
		downLimit <- struct{}{} //取出一个令牌（在管道里塞一个，分片任务完成之后再取出来）
	}
}

// 上传限速器。返回布尔，表示是否达到。
func upLimitGet() bool {
	if upLimit.upLimit == 0 { //如果没有限速
		return true
	} else {
		upLimit.mu.Lock()
		defer upLimit.mu.Unlock()
		if upLimit.upCount < upLimit.upLimit { //如果没有达到限速
			upLimit.upCount++
			return true
		} else { //如果达到限速
			return false
		}
	}
}

// 取出下载限速器的令牌
func downDown() { //这名字多少有点滑稽了，下载结束。
	if downLimit != nil {
		<-downLimit
	}
}

// 同理
func upDown() {
	if upLimit.upLimit != 0 {
		upLimit.mu.Lock()
		defer upLimit.mu.Unlock()
		upLimit.upCount--
	}
}
