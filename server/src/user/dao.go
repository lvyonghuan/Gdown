package user

import (
	"errors"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

//存放用户数据

var db *gorm.DB

func InitDB() {
	dsn := "root:42424242@tcp(127.0.0.1:3306)/gdown"
	database, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Println("mysql初始化错误:", err)
		return
	}
	db = database
}

func findUserByUsername(username string) (err error, user User) {
	err = db.Where("username=?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, User{}
	}
	return nil, user
}

func dbRegister(user User) (err error) {
	err = db.Create(&user).Error
	return err
}

func dbLogin(username string) (user User, err error) {
	err = db.Where("username=?", username).First(&user).Error
	return user, err
}
