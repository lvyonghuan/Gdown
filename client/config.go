package client

import (
	"github.com/spf13/viper"
	"log"
)

//客户端配置文件管理。
//客户端依赖配置文件运行。配置文件的格式为toml。使用viper。

// 配置文件结构
type config struct {
	ServiceAdr string `mapstructure:"service_adr"` //服务器地址
	UserName   string `mapstructure:"username"`    //用户名
	Password   string `mapstructure:"password"`    //密码
	token      string
}

var cfg config

// 读取配置文件
func readConfig() {
	viper.SetConfigName("config.toml")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./")
	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("读取配置文件错误:%v", err)
	}
	err = viper.Unmarshal(&cfg)
	if err != nil {
		log.Fatalf("解析配置文件错误:%v", err)
	}
}
