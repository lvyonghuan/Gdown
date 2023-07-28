package user

import (
	"fmt"
	"log"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
)

const tokenSecret = "MHcCAQEEIDmdyHY5w5w24RA1embdpeFjAORml1L9LhX2E3HFFHHhoAoGCCqGSM49AwEHoUQDQgAETZfbJRz5nkLy/mgwWUDURpiz3ZpMhEdw7SLQq1axt84zMSjGHvJOX2rcEzFsWo9E/GmVvdFUoDPNl1WIOQTIqg==" //token jwt秘钥

type User struct {
	Id       int
	Username string `json:"username"`
	Password string `json:"password"`
}

func Register(c *gin.Context) {
	//解析客户端发送的注册信息
	var reg User
	err := c.BindJSON(&reg)
	if err != nil {
		c.JSON(400, gin.H{
			"status":  400,
			"message": "注册信息格式错误",
		})
		log.Println(err)
		return
	}

	//在数据库中检查用户名是否已经被注册过
	err, u := findUserByUsername(reg.Username)
	if err != nil {
		c.JSON(500, gin.H{
			"status":  500,
			"message": "服务器内部错误",
		})
		log.Println(err)
		return
	} else if u != (User{}) {
		c.JSON(403, gin.H{
			"status":  403,
			"message": "用户名已经被注册",
		})
		return
	}

	//将用户信息填入数据库
	err = dbRegister(reg)
	if err != nil {
		c.JSON(500, gin.H{
			"status":  500,
			"message": "服务器内部错误",
		})
		log.Println(err)
		return
	}

	//返回成功消息
	c.JSON(200, gin.H{
		"status":  200,
		"message": "注册成功",
	})
}

// Login 登录
func Login(c *gin.Context) {
	//解析客户端发送的登录信息
	var login User
	err := c.BindJSON(&login)
	if err != nil {
		c.JSON(400, gin.H{
			"status":  400,
			"message": "登录信息格式错误",
			"token":   "",
		})
		log.Println(err)
		return
	}

	//连接数据库，进行用户信息核对
	user, err := dbLogin(login.Username)
	if err != nil {
		c.JSON(500, gin.H{
			"status":  500,
			"message": "服务器内部错误",
			"token":   "",
		})
		log.Println(err)
		return
	}
	if user == (User{}) {
		c.JSON(403, gin.H{
			"status":  403,
			"message": "用户名不存在",
			"token":   "",
		})
		log.Println(err)
		return
	}
	if user.Password != login.Password {
		c.JSON(403, gin.H{
			"status":  403,
			"message": "密码错误",
			"token":   "",
		})
		log.Println(err)
		return
	}

	//生成token
	token, err := generateToken()
	if err != nil {
		c.JSON(500, gin.H{
			"status":  500,
			"message": "服务器内部错误",
			"token":   "",
		})
		log.Println(err)
		return
	}

	//返回成功消息
	c.JSON(200, gin.H{
		"status":  200,
		"message": "登录成功",
		"token":   token,
	})
}

// 生成token
func generateToken() (string, error) {
	//使用jwt生成token
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims["exp"] = time.Now().Add(time.Hour * 24 * 7).Unix() //token有效期为7天
	claims["iat"] = time.Now().Unix()
	tokenString, err := token.SignedString([]byte(tokenSecret))
	return tokenString, err
}

// ParseToken 解析token
func ParseToken(tokenString string) (bool, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return false, err
	}
	if !token.Valid {
		return false, nil
	}
	expFloat64, ok := token.Claims.(jwt.MapClaims)["exp"].(float64)
	if !ok {
		return false, fmt.Errorf("无法解析 token 的 exp 字段")
	}
	exp := int64(expFloat64)

	if time.Now().Unix() > exp {
		return false, nil
	}
	return true, nil
}
