package cli

import (
	"log"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

//客户端配置文件管理。
//客户端依赖配置文件运行。配置文件的格式为toml。使用viper。

// 配置文件结构
type config struct {
	ServiceAdr string `mapstructure:"service_adr"` //服务器地址
	UserName   string `mapstructure:"username"`    //用户名
	Password   string `mapstructure:"password"`    //密码
	ClientPort int    `mapstructure:"client_port"` //指定客户端端口号
	DownRate   int    `mapstructure:"down_rate"`   //下载速度。为0时不限速，下同
	UpRate     int    `mapstructure:"up_rate"`     //上传速度
	token      string
}

var cfg config

// 读取配置文件
func ReadConfig() {
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

// 持续监视配置文件，实现热修改（在客户端登陆之后启用）
func hotReset() {
	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		err := viper.Unmarshal(&cfg)
		if err != nil {
			log.Println("解析配置文件错误:", err)
		}
		limit() //重写限速令牌桶
	})
}
