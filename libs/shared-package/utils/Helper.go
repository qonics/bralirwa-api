package utils

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	mathRand "math/rand"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"shared-package/proto"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unsafe"
	"web-service/model"

	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/viper"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var IsTestMode bool = false
var ctx = context.Background()

// var ctx = context.Background()
var SessionExpirationTime time.Duration = 1800
var CachePrefix string = "CACHE_MANAGER_"

const otpChars = "1234567890"
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
		log.Println("Logger service not connected: " + err.Error())
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
		log.Println("Logger service not responsed: " + err.Error())
	}
	log.Printf("Response: %s \n", r.GetResponse())
	return traceId
}

func USSDResponse(c *fiber.Ctx, networkCode string, action string, message string) error {
	if networkCode == "MTN2" {
		return c.JSON(fiber.Map{"action": action, "message": message})
	} else if networkCode == "MTN" {
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
		// fmt.Println("authentication failed, invalid token: ", err.Error(), "Data:", client)
		return nil, errors.New("authentication failed, invalid token")
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

// validate mtn phone number and return names when it is valid, and error if any
func ValidateMTNPhone(phoneNumber string, redis redis.Client) (string, error) {
	names := redis.Get(ctx, "name_"+phoneNumber).Val()
	if names != "" {
		return names, nil
	}
	//send http json request
	request, err := http.NewRequest("GET", fmt.Sprintf("%sapi/v1/momo/accountholder/information/%s", viper.GetString("MOMO_URL"), phoneNumber), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", viper.GetString("MOMO_KEY"))
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", err
	}
	if res, ok := result["status"].(float64); ok {
		if res != 200 {
			error_message, ok := result["message"].(string)
			if !ok {
				return "", errors.New("failed to validate phone number, err: " + error_message)
			}
			if error_message == "ACCOUNTHOLDER_NOT_FOUND" || error_message == "END_USER_SERVICE_DENIED" ||
				error_message == "RESOURCE_NOT_FOUND" || error_message == "AUTHORIZATION_RECEIVING_ACCOUNT_NOT_ACTIVE" {
				return "", errors.New("phone_error_momo")
			}
			return "", errors.New("failed to validate phone number, err: " + result["message"].(string))
		}
	} else {
		LogMessage("critical", "ValidateMTNPhone: failed to validate phone number, system error, body: "+string(body), "ussd-service")
		return "", errors.New("failed to validate phone number, system error")
	}
	names = result["firstname"].(string) + " " + result["lastname"].(string)
	//save names for 30 days
	redis.Set(ctx, "name_"+phoneNumber, names, time.Duration(720*time.Hour))
	return names, nil
}
func AirtelGetToken(redis redis.Client) (string, error) {
	fmt.Println("getting airtel token")
	reqBody, _ := json.Marshal(map[string]string{
		"client_id":     viper.GetString("AIRTEL_ID"),
		"client_secret": viper.GetString("AIRTEL_KEY"),
		"grant_type":    "client_credentials",
	})
	//send http json request
	request, err := http.NewRequest("POST", fmt.Sprintf("%s/auth/oauth2/token", viper.GetString("AIRTEL_URL")), bytes.NewBuffer([]byte(reqBody)))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", err
	}
	if res, ok := result["access_token"].(string); ok {
		redis.Set(ctx, "airtel_token", res, time.Duration(170*time.Second))
		fmt.Println("returning token: ", res)
		return res, nil
	} else {
		LogMessage("critical", "AirtelGetToken: failed to get token, system error, body: "+string(body), "ussd-service")
		return "", errors.New("failed to get token, system error")
	}
}
func ValidateAirtelPhone(phoneNumber string, redis redis.Client) (string, error) {
	//check if number is cached
	phoneNumber = strings.Replace(phoneNumber, "+", "", -1)
	phoneNumber = strings.Replace(phoneNumber, "250", "", -1)
	names := redis.Get(ctx, "name_"+phoneNumber).Val()
	if names != "" {
		return names, nil
	}
	token := redis.Get(ctx, "airtel_token").Val()
	var err error
	if token == "" {
		token, err = AirtelGetToken(redis)
		fmt.Println("After fetching token: ", token, err)
		if err != nil {
			return "", err
		}
	}
	fmt.Println("validating airtel phone")
	phoneNumber = strings.Replace(phoneNumber, "+", "", -1)
	phoneNumber = strings.Replace(phoneNumber, "250", "", -1)
	//send http json request
	request, err := http.NewRequest("GET", fmt.Sprintf("%s/standard/v1/users/%s", viper.GetString("AIRTEL_URL"), phoneNumber), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Country", "RW")
	request.Header.Set("X-Currency", "RWF")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	fmt.Println(string(body))
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", err
	}
	if res, ok := result["status"].(map[string]any); ok {
		if res["code"].(string) == "200" {
			data := result["data"].(map[string]any)
			names := data["first_name"].(string) + " " + data["last_name"].(string)
			//save names for 30 days
			redis.Set(ctx, "name_"+phoneNumber, names, time.Duration(720*time.Hour))
			return names, nil
		} else {
			error_message, ok := res["message"].(string)
			if !ok {
				return "", errors.New("failed to validate AIRTEL phone number, err: " + error_message)
			}
			if res["response_code"].(string) == "DP02200000000" {
				return "", errors.New("phone_error_momo")
			}
			return "", errors.New("failed to validate AIRTEL phone number, err: " + result["message"].(string))
		}
	} else {
		LogMessage("critical", "ValidateMTNPhone: failed to validate phone number, system error, body: "+string(body), "ussd-service")
		return "", errors.New("failed to validate AIRTEL phone number, system error")
	}
}

// send sms, return message_id on success and error if any
func SendSMS(DB *pgxpool.Pool, phoneNumber string, message string, senderName string, serviceName string, messageType string, customerId *int, redis *redis.Client) (string, error) {
	//skip this if it is test
	if IsTestMode {
		return "TEST_SMS_ID", nil
	}
	fmt.Println("sending sms to ", phoneNumber)
	networkOperator := "MTN"
	if strings.HasPrefix(phoneNumber, "073") || strings.HasPrefix(phoneNumber, "072") {
		networkOperator = "AIRTEL"
	}
	payload := map[string]interface{}{
		"sender_id":        senderName,
		"phone":            phoneNumber,
		"message":          message,
		"network_operator": networkOperator,
	}
	jsonData, _ := json.Marshal(payload)
	//send http json request
	request, err := http.NewRequest("POST", viper.GetString("sms_service_url"), bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(request)
	error_message := ""
	if err != nil {
		error_message = err.Error()
	}
	if resp == nil {
		fmt.Println("failed to send sms, no response, err:", error_message, "body:", string(jsonData))
		LogMessage("critical", "SendSMS: ailed to send sms, no response", serviceName)
		return "", errors.New("failed to send sms, no response")
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		error_message = err.Error()
	}
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		error_message = err.Error()
	}
	messageId := ""
	if res, ok := result["status"].(string); ok {
		if res != "success" {
			error_message = "failed to send sms, err: " + result["message"].(string)
		} else {
			messageId = result["message_id"].(string)
			// redis.Set(ctx, "UPDATE_SMS_BALANCE", "1", 30*time.Minute)
		}
	} else {
		LogMessage("critical", "SendSMS: failed to send sms, system error, body: "+string(body), serviceName)
		error_message = "failed to send sms, system error"
	}
	status := "SENT"
	if error_message != "" {
		status = "FAILED"
	}
	if messageType == "password" || messageType == "reset_password_otp" || messageType == "account_password" {
		message = "Message content is hidden for security reasons"
	}
	_, err = DB.Exec(ctx, "INSERT INTO sms (customer_id, message, phone, type, status, message_id, credit_count, error_message) VALUES ($1, $2, $3, $4, $5, $6, 0, $7)",
		customerId, message, phoneNumber, messageType, status, messageId, error_message)
	if err != nil {
		LogMessage("critical", "SendSMS: failed to save sms, err: "+err.Error(), serviceName)
	}
	return messageId, nil
}

// send sms, return message_id on success and error if any
func SMSBalance(DB *pgxpool.Pool, serviceName string, redis *redis.Client) (int, error) {
	//skip this if it is test
	if IsTestMode {
		return 10, nil
	}
	localCredit := redis.Get(ctx, "SMS_BALANCE")
	forceUpdate := redis.Get(ctx, "UPDATE_SMS_BALANCE")
	if forceUpdate.Val() == "0" {
		localCreditInt, _ := strconv.Atoi(localCredit.Val())
		return localCreditInt, nil
	}
	//send http json request
	request, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/balance/send-sms/rw", viper.GetString("SMS_URL")), nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("x-api-key", viper.GetString("SMS_KEY"))
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(request)
	error_message := ""
	if err != nil {
		error_message = err.Error()
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		error_message = err.Error()
	}
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		error_message = err.Error()
	}
	if error_message != "" {
		return 0, errors.New(error_message)
	}
	if res, ok := result["status"].(float64); ok {
		if res != 200 {
			error_message = "failed to get sms balance, err: " + result["message"].(string)
		} else {
			data := result["data"].(map[string]interface{})
			credit := int(data["credit"].(float64))
			redis.Set(ctx, "UPDATE_SMS_BALANCE", "0", 30*time.Minute)
			redis.Set(ctx, "SMS_BALANCE", credit, 30*time.Minute)
			return credit, nil
		}
	} else {
		LogMessage("critical", "SMSBalance: failed to get sms balance, system error, body: "+string(body), serviceName)
		error_message = "failed to get sms balance, system error"
	}
	return 0, errors.New(error_message)
}

// IsStrongPassword checks if the given password is strong from validator, you will register customer validator and use it .
//
// # Minimum 8 characters
//
// # Contains at least one digit
//
// # Contains at least one uppercase letter
//
// # Contains at least one lowercase letter
//
// # Contains at least one special character
//
// # No three successive similar special characters, text, or numbers
//
// Args:
//
//	password (validator.FieldLevel): a password you want to validate
//
// Returns:
//
//	bool: true for strong password and false for weak password
//
// Examples:
//
//	IsStrongPassword("MyStr0ngP@ssw0rd") // true
//
//	IsStrongPassword("weak") // false
func IsStrongPassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()
	if len(password) < 8 {
		return false
	}

	hasDigit := false
	hasUpper := false
	hasLower := false
	hasSpecial := false

	for i, char := range password {
		switch {
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}

		// Check for three successive similar characters
		if i >= 2 && password[i] == password[i-1] && password[i-1] == password[i-2] {
			return false
		}
	}
	return hasDigit && hasUpper && hasLower && hasSpecial
}
func GenerateOTP(length int) (string, error) {
	buffer := make([]byte, length)
	_, err := rand.Read(buffer)
	if err != nil {
		return "", err
	}
	otpCharsLength := len(otpChars)
	for i := 0; i < length; i++ {
		buffer[i] = otpChars[int(buffer[i])%otpCharsLength]
	}
	return string(buffer), nil
}
func GenerateHtmlTemplate(filename string, emailData any) (string, error) {
	filename = strings.Replace(filename, ".html", "", -1)
	filepath := fmt.Sprintf("/app/templates/%s.html", filename)
	if IsTestMode {
		filepath = fmt.Sprintf("../templates/%s.html", filename)
	}
	tmpl, err := template.ParseFiles(filepath)
	if err != nil {
		return "", errors.New("error parsing template: " + err.Error())
	}

	// Render the template to a string
	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		return "", errors.New("error rendering template: " + err.Error())
	}
	return body.String(), nil
}
func SendEmail(to string, subject string, body string, serviceName string) string {
	//skip this if it is test
	if IsTestMode {
		return "Email sent"
	}
	//TODO: send email
	return "Email sent"
}

// RecordActivityLog inserts an activity log into the activity_logs table
func RecordActivityLog(db *pgxpool.Pool, log ActivityLog, serviceName string, extra *map[string]interface{}) error {
	query := `
    INSERT INTO activity_logs (user_id, activity_type, status, description, ip_address, user_agent, extra, created_at, updated_at)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `
	extraData, err := json.Marshal(extra)
	if err != nil {
		return fmt.Errorf("could not marshal extra data: %w", err)
	}
	_, err = db.Exec(
		ctx,
		query,
		log.UserID,
		log.ActivityType,
		log.Status,
		log.Description,
		log.IPAddress,
		log.UserAgent,
		extraData,
		time.Now(),
		time.Now(),
	)

	if err != nil {
		LogMessage("critical", "RecordActivityLog: could not insert activity log: "+err.Error(), serviceName)
		return fmt.Errorf("could not insert activity log: %w", err)
	}

	return nil
}

// getOdds returns the probability of getting true based on the hour of the day.
func getOdds(hour int) int {
	switch {
	case hour >= 0 && hour < 7:
		return 5
	case hour >= 7 && hour < 12:
		return 2
	case hour >= 12 && hour < 16:
		return 1
	case hour >= 16 && hour < 22:
		return 1
	default:
		return 1
	}
}

// generateBoolWithOdds returns true or false based on the odds determined by the current time.
func GenerateBoolWithOdds(rng *mathRand.Rand) bool {
	currentTime := time.Now()
	odds := getOdds(currentTime.Hour())
	return rng.Intn(100) < odds
}

func BuildQueryFilter(filter map[string]interface{}, args *[]interface{}) (string, int) {
	i := 1
	query := ""
	for key, value := range filter {
		if value != "" {
			if len(query) != 0 {
				query += " and "
			}
			//check if key contains any comparison operator (>, <, >=, <=, !=, =)
			if strings.ContainsAny(key, "<>=") {
				query += fmt.Sprintf("%s $%d", key, i)
			} else {
				query += fmt.Sprintf("%s = $%d", key, i)
			}
			*args = append(*args, value)
			i++
		}
	}
	if len(query) != 0 {
		query = " where " + query
	}
	return query, i
}
func ValidateDateRanges(startDate string, endDate *string) error {
	var startDateVal time.Time
	if len(startDate) != 0 {
		startDateVall, err := time.Parse("2006-01-02", startDate)
		if err != nil {
			return errors.New("invalid start date provided")
		}
		startDateVal = startDateVall
	}
	if len(*endDate) != 0 {
		endDateVal, err := time.Parse("2006-01-02", *endDate)
		if err != nil {
			return errors.New("invalid end date provided")
		}
		//check if end date is after start date
		if len(startDate) != 0 {
			if endDateVal.Before(startDateVal) {
				return errors.New("end date should be after start date")
			}
		}
		//add one day to include the end date
		endDateVal = endDateVal.AddDate(0, 0, 1)
		*endDate = endDateVal.Format("2006-01-02")
	}
	return nil
}

func MoMoCredit(amount int, phone string, trxId string, code string) (string, error) {
	if IsTestMode {
		return "TEST_SMS_ID", nil
	}
	payload := map[string]interface{}{
		"amount":        amount,
		"account_no":    phone,
		"transactionId": viper.GetString("MOMO_TRX_PREFIX") + trxId,
		"currency":      "RWF",
		"payment_type":  "momo",
		"message":       "BRALIRWA Coca Cola campaign PRIZE, CODE: " + code,
	}
	jsonData, _ := json.Marshal(payload)
	//send http json request
	request, err := http.NewRequest("POST", fmt.Sprintf("%sapi/v1/momo/transfer", viper.GetString("MOMO_URL")), bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", viper.GetString("MOMO_KEY"))
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	fmt.Println(string(body))
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", err
	}
	if res, ok := result["status"].(float64); ok {
		if res != 200 {
			error_message, ok := result["message"].(string)
			if !ok {
				return "", errors.New("failed to credit winner, err: " + error_message)
			}
			return "", errors.New("failed to credit winner, err: " + error_message)
		}
	} else {
		LogMessage("critical", "MoMoCredit: failed to credit winner, system error, Code: "+code+" Phone: "+phone+" Amount: "+strconv.Itoa(amount)+" TrxId: "+trxId, "web-service")
		return "", errors.New("failed to credit winner, system error. Code: " + code)
	}
	return result["momoRef"].(string), nil
}

func AirtelCredit(amount int, phone string, trxId string, code string, redis redis.Client) (string, error) {
	if IsTestMode {
		return "TEST_SMS_ID", nil
	}
	token := redis.Get(ctx, "airtel_token").Val()
	var err error
	if token == "" {
		token, err = AirtelGetToken(redis)
		fmt.Println("After fetching token: ", token, err)
		if err != nil {
			return "", err
		}
	}
	phone = strings.Replace(phone, "250", "", -1)
	payload := map[string]interface{}{
		"payee": map[string]interface{}{
			"msisdn": phone,
		},
		"pin":       viper.GetString("AIRTEL_PIN"),
		"amount":    amount,
		"reference": code,
		"transaction": map[string]any{
			"id":     viper.GetString("MOMO_TRX_PREFIX") + trxId,
			"amount": amount,
		},
	}
	jsonData, _ := json.Marshal(payload)
	fmt.Println("Airtel Raw payload: ", string(jsonData))
	//send http json request
	request, err := http.NewRequest("POST", fmt.Sprintf("%s/standard/v1/disbursements/", viper.GetString("AIRTEL_URL")), bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Country", "RW")
	request.Header.Set("X-Currency", "RWF")
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	fmt.Println(string(body))
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", err
	}
	if res, ok := result["status"].(map[string]any); ok {
		if res["code"] != "200" {
			error_message, ok := res["message"].(string)
			if !ok {
				return "", errors.New("airtel: failed to credit winner, err: " + error_message)
			}
			return "", errors.New("airtel: failed to credit winner, err: " + error_message)
		}
	} else {
		LogMessage("critical", "AirtelCredit: failed to credit winner, system error, Code: "+code+" Phone: "+phone+" Amount: "+strconv.Itoa(amount)+" TrxId: "+trxId, "web-service")
		return "", errors.New("airtel: failed to credit winner, system error. Code: " + code)
	}
	if data, ok := result["data"].(map[string]any); ok {
		if transaction, ok := data["transaction"].(map[string]any); ok {
			if transaction["status"].(string) == "TS" {
				return transaction["reference_id"].(string), nil
			} else {
				return "", errors.New("airtel: failed to credit winner, transaction status: " + transaction["status"].(string))
			}
		}
	}
	return "", errors.New("airtel: failed to credit winner, undefined error")
}

// / LoadPublicKey loads an RSA public key from PEM format
func LoadPublicKey(pemStr string) (*rsa.PublicKey, error) {
	pemStr = "-----BEGIN PUBLIC KEY-----\n" + pemStr + "\n-----END PUBLIC KEY-----"
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return rsaPub, nil
}

// EncryptPIN encrypts a PIN using RSA-OAEP with SHA-256
func EncryptData(data string, publicKey *rsa.PublicKey) (string, error) {
	// Convert PIN to bytes
	plaintext := []byte(data)

	// Encrypt using RSA-OAEP with SHA-256
	ciphertext, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		publicKey,
		plaintext,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("encryption error: %v", err)
	}
	// Encode to base64 for transmission
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}
func encryptWithPublicKey(data []byte, publicKey *rsa.PublicKey) (string, error) {
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader,
		publicKey,
		data)
	if err != nil {
		return "", err
	}
	ciphertextStr := base64.StdEncoding.EncodeToString(ciphertext)
	return ciphertextStr, nil
}

// EncryptJSONPayload encrypts a JSON payload using AES-256-GCM, and encrypts the AES key using RSA-OAEP.
// returns the encrypted AES key and IV, and ciphertext as base64-encoded strings.
func EncryptJSONPayload(plaintext []byte, publicKey *rsa.PublicKey) (string, string, error) {

	// Generate a random 256-bit AES key
	aesKey := make([]byte, 32)
	_, err := rand.Read(aesKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate AES key: %v", err)
	}

	// Generate a random 96-bit AES IV
	iv := make([]byte, 12)
	_, err = rand.Read(iv)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate AES IV: %v", err)
	}

	// Encrypt the plaintext using AES-256-GCM
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create AES cipher: %v", err)
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", fmt.Errorf("failed to create GCM: %v", err)
	}
	ciphertext := aesgcm.Seal(nil, iv, plaintext, nil)

	// Encrypt the AES key using RSA-OAEP with SHA-256
	encryptedAESKey, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		publicKey,
		aesKey,
		nil,
	)
	if err != nil {
		return "", "", fmt.Errorf("failed to encrypt AES key: %v", err)
	}

	// Encode the encrypted AES key and IV for the header
	encryptedAESKeyStr := base64.StdEncoding.EncodeToString(encryptedAESKey)
	encryptedIVStr := base64.StdEncoding.EncodeToString(iv)
	headerValue := fmt.Sprintf("%s:%s", encryptedAESKeyStr, encryptedIVStr)

	// Encode the ciphertext for the payload
	ciphertextStr := base64.StdEncoding.EncodeToString(ciphertext)

	return headerValue, ciphertextStr, nil
}

func MoMoCheckStatus(trxId string) (string, error) {
	if IsTestMode {
		return "TEST_SMS_ID", nil
	}
	trxId = viper.GetString("MOMO_TRX_PREFIX") + trxId
	//send http json request
	request, err := http.NewRequest("GET", fmt.Sprintf("%sapi/v1/momo/transactionstatus/%s", viper.GetString("MOMO_URL"), trxId), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", viper.GetString("MOMO_KEY"))
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	fmt.Println(string(body))
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", err
	}
	if res, ok := result["status"].(float64); ok {
		if res != 200 {
			error_message, ok := result["message"].(string)
			if !ok {
				return "", errors.New("failed to check momo status, err: " + error_message + ", trxId: " + trxId)
			}
			return "", errors.New("failed to check momo status, err: " + error_message + ", trxId: " + trxId)
		}
	} else {
		LogMessage("critical", "MoMoCheckStatus: failed to check mtn status, system error, trxId: "+trxId+". response body: "+string(body), "web-service")
		return "", errors.New("failed to check status, system error")
	}
	return result["momoRef"].(string), nil
}

func AirtelCheckStatus(trxId string, redis redis.Client) (string, error) {
	if IsTestMode {
		return "TEST_SMS_ID", nil
	}
	token := redis.Get(ctx, "airtel_token").Val()
	var err error
	if token == "" {
		token, err = AirtelGetToken(redis)
		fmt.Println("After fetching token: ", token, err)
		if err != nil {
			return "", err
		}
	}
	trxId = viper.GetString("MOMO_TRX_PREFIX") + trxId
	//send http json request
	request, err := http.NewRequest("GET", fmt.Sprintf("%s/standard/v1/disbursements/%s", viper.GetString("AIRTEL_URL"), trxId), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Country", "RW")
	request.Header.Set("X-Currency", "RWF")
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	fmt.Println(string(body))
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", err
	}
	if res, ok := result["status"].(map[string]any); ok {
		if res["code"].(string) != "200" {
			error_message, ok := res["message"].(string)
			if !ok {
				return "", errors.New("failed to check airtel status,, err: " + error_message)
			}
			return "", errors.New("failed to check airtel status, err: " + error_message)
		}
	} else {
		LogMessage("critical", "MoMoCheckStatus: failed to check airtel status, system error, trxId: "+trxId+". response body: "+string(body), "web-service")
		return "", errors.New("failed to check status, system error")
	}
	if data, ok := result["data"].(map[string]any); ok {
		if transaction, ok := data["transaction"].(map[string]any); ok {
			if transaction["status"].(string) == "TS/200" {
				return transaction["id"].(string), nil
			} else if transaction["status"].(string) == "TF" {
				return "", errors.New("fail: " + transaction["message"].(string))
			} else {
				return "", errors.New("airtel: failed to check airtel status, transaction status: " + transaction["status"].(string))
			}
		}
	}
	return "", errors.New("airtel: failed to check airtel status, undefined error")
}

func ExportToExcel(fileName string, sheetName string, data any) ([]byte, error) {
	// Ensure the input is a slice of structs
	sliceValue := reflect.ValueOf(data)
	if sliceValue.Kind() != reflect.Slice {
		return nil, fmt.Errorf("data must be a slice")
	}

	// Create a new Excel file
	f := excelize.NewFile()
	if sheetName == "" {
		sheetName = "Sheet1"
	}
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(index)

	// Check if the slice has elements
	if sliceValue.Len() == 0 {
		return nil, fmt.Errorf("data slice is empty")
	}

	// Get the struct type from the first element
	firstElem := sliceValue.Index(0)
	if firstElem.Kind() != reflect.Struct {
		return nil, fmt.Errorf("slice must contain structs")
	}
	elemType := firstElem.Type()

	// Write headers based on struct field names
	for i := 0; i < elemType.NumField(); i++ {
		header := elemType.Field(i).Name
		colName, _ := excelize.ColumnNumberToName(i + 1)
		f.SetCellValue(sheetName, colName+"1", header)
	}

	// Write the data rows
	for rowIndex := 0; rowIndex < sliceValue.Len(); rowIndex++ {
		rowValue := sliceValue.Index(rowIndex)
		for colIndex := 0; colIndex < elemType.NumField(); colIndex++ {
			cellValue := rowValue.Field(colIndex).Interface()
			// Check for null (pointer) and empty strings
			if strPtr, ok := cellValue.(*string); ok {
				if strPtr == nil || *strPtr == "" {
					cellValue = "" // Treat as empty string for null or empty string pointer
				} else {
					cellValue = *strPtr
				}
			}
			colName, _ := excelize.ColumnNumberToName(colIndex + 1)
			f.SetCellValue(sheetName, colName+fmt.Sprint(rowIndex+2), cellValue)
		}
	}
	// Save the file
	// return f.SaveAs(fileName)
	// Save the file to a buffer
	buf := new(bytes.Buffer)
	if err := f.Write(buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
