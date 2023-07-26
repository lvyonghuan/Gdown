package client

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

//客户端注册登录

// 注册
func register() {
	//获取用户注册信息
	type register struct {
		Username string `json:"username"`
		Password string `json:"Password"`
	}
	var reg register
	reg.Username = cfg.UserName
	reg.Password = cfg.Password

	//序列化用户注册信息
	buf, err := json.Marshal(reg)
	if err != nil {
		log.Println("序列化用户注册信息错误:", err)
		return
	}

	//发送注册信息
	req, err := http.NewRequest("POST", "http://"+cfg.ServiceAdr+"/user/register", bytes.NewBuffer(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(len(buf)))
	req.Header.Set("User-Agent", "GDown")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("发送注册信息错误:", err)
		return
	}

	//解析服务器回传信息
	defer resp.Body.Close()
	var response struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Println("解析服务器回传信息错误:", err)
		return
	}
	if response.Status != http.StatusOK {
		log.Println("注册失败:", response.Message)
		return
	}
	log.Println("注册成功")
}

// 登录
func login() {
	//获取用户登录信息
	type loginInfo struct {
		Username string `json:"username"`
		Password string `json:"Password"`
	}
	var login loginInfo
	login.Username = cfg.UserName
	login.Password = cfg.Password

	//序列化用户登录信息
	buf, err := json.Marshal(login)
	if err != nil {
		log.Println("序列化用户登录信息错误:", err)
		return
	}

	//发送登录信息
	req, err := http.NewRequest("GET", "http://"+cfg.ServiceAdr+"/user/login", bytes.NewBuffer(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(len(buf)))
	req.Header.Set("User-Agent", "GDown")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("发送登录信息错误:", err)
		return
	}

	//解析服务器回传信息
	defer resp.Body.Close()
	var response struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Token   string `json:"token"`
	}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Println("解析服务器回传信息错误:", err)
		return
	}
	if response.Status != http.StatusOK {
		log.Println("登录失败:", response.Message)
		return
	}

	//保存token
	cfg.token = response.Token
}
