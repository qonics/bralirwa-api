package helper

import (
	"auth-service/config"
	"auth-service/model"
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"
	"unsafe"

	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

var ctx = context.Background()
var SessionExpirationTime time.Duration = 1800
var CachePrefix string = "CACHE_MANAGER_"

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func InitializeViper() {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetConfigType("yml")
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading config file, %s", err)
	}
}

func RandString(n int) string {
	var src = rand.NewSource(time.Now().UnixNano())
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return *(*string)(unsafe.Pointer(&b))
}

func GetUniqueSecret(key *string) (string, string) {
	keyCode := RandString(12)
	if key != nil {
		keyCode = *key
	}
	secret := fmt.Sprintf("%s.%s", os.Getenv("secret"), keyCode)
	return keyCode, secret
}
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}
func SaveActivityLog(eventType string, activity string, ipAddress string, agent string, operator uint, table string, id string) {
	resp := config.DB.Create(&model.ActivityLogs{
		EventType: eventType,
		Activity:  activity,
		IpAddress: ipAddress,
		Operator:  operator,
		Agent:     agent,
		Type:      table,
		Extra:     fmt.Sprintf("{\"id\":\"%s\"}", id),
	})
	if resp.Error != nil {
		Warning("Unable to save log: " + resp.Error.Error())
	}
}

func Testss() {

}
