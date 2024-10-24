package utils

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	mathRand "math/rand"
	"os"
	"reflect"
	"regexp"
	"shared-package/proto"
	"strings"
	"time"
	"unsafe"
	"web-service/model"

	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var IsTestMode bool = false
var ctx = context.Background()

// var ctx = context.Background()
var SessionExpirationTime time.Duration = 1800
var CachePrefix string = "CACHE_MANAGER_"

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

// Define the LogLevel type as a string
type LogLevel string

const (
	INFO     LogLevel = "INFO"
	DEBUG    LogLevel = "DEBUG"
	ERROR    LogLevel = "ERROR"
	CRITICAL LogLevel = "CRITICAL"
)

type Logger struct {
	LogLevel    LogLevel
	Message     string
	ServiceName string
}

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
	// Map the environment variable POSTGRES_DB_PASSWORD to the config path postgres_db.password
	viper.BindEnv("postgres_db.password", "POSTGRES_DB_PASSWORD")
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
		return c.JSON(fiber.Map{"action": action, "message": message})
	} else if networkCode == "MTN2" {
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
	LogMessage("error", "USSDResponse: Invalid network code, code:"+networkCode, "ussd-service")
	return errors.New("invalid network code")
}

func Localize(localizer *i18n.Localizer, messageID string, templateData map[string]interface{}) string {
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
	if err != nil {
		LogMessage("error", "Localize: "+err.Error(), "ussd-service")
		return messageID
	}
	return msg
}

// check if item Exist in string slice
func ContainsString(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// return json response and save logs if logger container 1 or more data
func JsonErrorResponse(c *fiber.Ctx, responseStatus int, message string, logger ...Logger) error {
	c.SendStatus(responseStatus)
	traceId := ""
	//save logs if it is available
	for _, log := range logger {
		logId := ""
		if !IsTestMode {
			logId = LogMessage(string(log.LogLevel), log.Message, log.ServiceName, traceId)
		} else {
			fmt.Println(log.Message)
		}
		//update traceId once it is empty only, then other logs will use that traceId
		if traceId == "" {
			traceId = logId
		}
	}
	publicMessage := message
	//never show actual system error as per AOWSAP code: AOW-5001 (Internal Server Error (Public-Facing Generic Message))
	if responseStatus >= 500 {
		if len(message) < 3 {
			publicMessage = "Our apologies, something went wrong. Please try again in a little while. Trace_id: " + traceId
		} else {
			publicMessage = fmt.Sprintf("%s, Sorry for the inconvenience! Please give it another go in a bit. Trace_id: %s", message, traceId)
		}
	} else if traceId != "" {
		publicMessage = fmt.Sprintf("%s Trace_id: %s", message, traceId)
	}
	return c.JSON(fiber.Map{"status": responseStatus, "message": publicMessage, "trace_id": traceId})
}
func ValidateString(s string, ignoreChars ...string) bool {
	if s == "" {
		return false
	}

	disallowedChars := `'£$%&*()}{#~?><>,/|=_+¬`
	for _, char := range ignoreChars {
		disallowedChars = strings.Replace(disallowedChars, char, "", -1)
	}

	disallowedPattern := "[" + regexp.QuoteMeta(disallowedChars) + "]"
	re := regexp.MustCompile(disallowedPattern)
	return re.MatchString(s)
}

// loop through struct value and validate each for unwanted special characters
//
// Args:
//
//	data (interface{}): a struct you want to validate
//	ignoreChars ([]string) (optional): List of ignored characters
//	ignoredKeys ([]string) (optional): List of ignored keys, and you must pass ignoreChars as an empty slice if it is not needed
//
// Returns:
//
//	map[string]bool: a map of keys with invalid special characters and with true as value
//
// Examples:
//
//	ValidateStruct(data)
//	ValidateStruct(data, []string{"=","\\"}) // exclude = and \
//	ValidateStruct(data, []string{}, []string{"Password"}) // exclude Password key from validation
func ValidateStruct(data interface{}, extra ...[]string) map[string]bool {
	results := make(map[string]bool)
	val := reflect.ValueOf(data).Elem()
	ignoredKeys, ignoreChars := []string{}, []string{}
	if len(extra) > 0 {
		ignoreChars = extra[0]
	}
	if len(extra) > 1 {
		ignoredKeys = extra[1]
	}
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		keyName := val.Type().Field(i).Name
		if ContainsString(ignoredKeys, keyName) {
			continue
		}
		if field.Kind() == reflect.String {
			str := field.String()
			valid := ValidateString(str, ignoreChars...)
			if valid {
				results[keyName] = valid
			}
		}
	}
	return results
}

// Genereate a message from ValidateStruct response
//
// Args:
//
//	data (map[string]bool): The response returned from ValidateStruct.
//
// Returns:
//
//	*string: An error message from validation map.
func ValidateStructText(data map[string]bool) *string {
	text := ""
	for a := range data {
		text += fmt.Sprintf("%s contains unsupported characters<br />", a)
	}
	if text == "" {
		return nil
	}
	return &text
}
func SecurePath(c *fiber.Ctx, redis *redis.Client) (*model.UserProfile, error) {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		authHeader = c.Get("authorization")
	}
	authHeader = strings.ReplaceAll(authHeader, "Bearer ", "")
	responseStatus := fiber.StatusUnauthorized
	if authHeader == "" {
		c.SendStatus(responseStatus)
		return nil, errors.New("unauthorized: You are not allowed to access this resource")
	}
	client := []byte(redis.Get(ctx, authHeader).Val())
	if client == nil {
		isLogout := c.Locals("isLogout")
		if isLogout != nil && isLogout.(bool) {
			c.SendStatus(fiber.StatusOK)
			return nil, errors.New("already logged out")
		}
		c.SendStatus(responseStatus)
		return nil, errors.New("token not found or expired")
	}
	var logger model.UserProfile
	err := json.Unmarshal(client, &logger)
	if err != nil {
		c.SendStatus(responseStatus)
		fmt.Println("authentication failed, invalid token: ", err.Error(), "Data:", client)
		return nil, errors.New("authentication failed, invalid token." + err.Error())
	}

	redis.Expire(ctx, authHeader, time.Duration(SessionExpirationTime*time.Minute))
	logger.AccessToken = authHeader
	return &logger, nil
}

// Custom function to validate with regex provided in struct tag
func RegexValidation(fl validator.FieldLevel) bool {
	param := fl.Param() // Get the regex pattern from the struct tag
	regex := regexp.MustCompile(param)
	return regex.MatchString(fl.Field().String())
}
func IsErrDuplicate(err error) (bool, string) {
	if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
		keyName := ""
		key := strings.Split(err.Error(), "\"")[1]
		switch key {
		case "prize_category_name_key":
			keyName = "Category name"
		case "unique_code_prize":
			keyName = "code and prize type"
		case "unique_customer_prize":
			keyName = "customer and prize type"
		case "users_phone_key":
			keyName = "phone"
		case "users_email_key":
			keyName = "email"
		default:
			keyName = key
		}
		return true, keyName
	}
	return false, ""
}

func IsForeignKeyErr(err error) (bool, string) {
	if strings.Contains(err.Error(), "violates foreign key constraint") {
		keyName := ""
		key := strings.Split(err.Error(), "\"")[3]
		switch key {
		case "prize_type_prize_category_id_fkey":
			keyName = "Category id"
		default:
			keyName = key
		}
		return true, keyName
	}
	return false, ""
}
func GenerateRandomNumber(length int) int {
	mathRand.New(mathRand.NewSource(time.Now().UnixNano()))
	return mathRand.Intn(length) + 1
}
func GenerateRandomCapitalLetter(length int) string {
	mathRand.Seed(time.Now().UnixNano()) // Seed the random number generator with the current time
	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = letters[mathRand.Intn(len(letters))]
	}
	return string(result)
}
