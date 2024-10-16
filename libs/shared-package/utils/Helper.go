package utils

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	mathRand "math/rand"
	"os"
	"shared-package/proto"
	"time"
	"unsafe"

	"github.com/gofiber/fiber/v2"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var IsTestMode bool = false
var ctx = context.Background()
var SessionExpirationTime time.Duration = 1800
var CachePrefix string = "CACHE_MANAGER_"

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func RandString(n int) string {
	var src = mathRand.NewSource(time.Now().UnixNano())
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

// preventing application from crashing abruptly. use defer PanicRecover() on top of the codes that may cause panic
func PanicRecover() {
	if r := recover(); r != nil {
		log.Println("Recovered from panic: ", r)
	}
}
func InitializeViper(configName string, configType string) {
	viper.SetConfigName(configName)
	if IsTestMode {
		fmt.Println("Running in Test mode...")
		viper.AddConfigPath("../") // Adjust the path for test environment
	} else {
		// Normal mode configuration
		viper.AddConfigPath("/app") // Adjust the path for production environment
	}
	viper.AutomaticEnv()
	viper.SetConfigType(configType)
	if viper.AllKeys() == nil {
		if err := viper.ReadInConfig(); err != nil {
			log.Fatal("Error reading config file, ", err)
		}
	} else {
		if err := viper.MergeInConfig(); err != nil {
			log.Fatalf("Error reading config file 2, %s", err)
		}
	}
}
func GenerateCSRFToken() string {
	token := make([]byte, 32)
	_, err := rand.Read(token)
	if err != nil {
		log.Panic("Unable to generate CSRF Token")
	}
	return hex.EncodeToString(token)
}
func LogMessage(logLevel string, message string, service string, forcedTraceId ...string) string {
	fmt.Println(message)
	conn, err := grpc.Dial("logger-service:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Logger service not connected: " + err.Error())
	}
	defer conn.Close()
	client := proto.NewLoggerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	traceId := RandString(12)
	//manually set log trace id
	if forcedTraceId != nil && forcedTraceId[0] != "" {
		traceId = forcedTraceId[0]
	}
	r, err := client.Log(ctx, &proto.LogRequest{LogLevel: logLevel, LogTime: time.Now().Format(time.DateTime),
		ServiceName: service, Message: message, Identifier: traceId})
	if err != nil {
		log.Fatal("Logger service not responsed: " + err.Error())
	}
	log.Printf("Response: %s", r.GetResponse())
	return traceId
}

func USSDResponse(c *fiber.Ctx, networkCode string, action string, message string) error {
	if networkCode == "MTN" {
		c.Set("Content-Type", "text/plain")
		c.Set("Freeflow", action)
		c.Set("Cache-Control", "max-age=0")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "-1")
		c.Set("Content-Length", fmt.Sprintf("%v", len(message)))
		c.SendStatus(200)
		c.SendString(message)
		return nil
	} else if networkCode == "AIRTEL" {
		c.Set("Content-Type", "text/plain")
		c.Set("Freeflow", action)
		c.Set("Cache-Control", "max-age=0")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "-1")
		c.Set("Content-Length", fmt.Sprintf("%v", len(message)))
		c.SendStatus(200)
		c.SendString(message)
		return nil
	}
	LogMessage("error", "USSDResponse: Invalid network code, "+networkCode, "ussd-service")
	return errors.New("invalid network code")
}

func Localize(localizer *i18n.Localizer, messageID string, templateData map[string]interface{}) string {
	return localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
}
