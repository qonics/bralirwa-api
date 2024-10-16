package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"shared-package/utils"
	"strings"
	"time"
	"unicode/utf8"
	"ussd-service/config"
	"ussd-service/model"

	"github.com/BurntSushi/toml"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/viper"
	"golang.org/x/text/language"
)

var Validate = validator.New()
var ctx = context.Background()
var systemError = "System error. Please try again later."

// TODO: load this from hashicorp vault
var encryptionKey = "example-encryption-key"

const USSD_MAX_LENGTH = 160 // Example value, adjust as needed

type USSDFlow struct {
	// Define fields and methods for your model
}

var bundle *i18n.Bundle
var lang = "en" // Default language
var localizer *i18n.Localizer

func init() {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	_, err := bundle.LoadMessageFile("/app/locales/ussd.en.toml")
	if err != nil {
		fmt.Println("Error loading translations:", err)
	}
	// bundle.LoadMessageFile("ussd.rw.toml")
	// ... add more translations as needed
}
func loadLocalizer(lang string) *i18n.Localizer {
	return i18n.NewLocalizer(bundle, lang)
}
func (u *USSDFlow) SetNextData(sessionId string, nextData string) error {
	// Implement the logic to store nextData in the database
	return nil
}
func ServiceStatusCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": 200, "message": "Welcome to the Lottery USSD API service. This service is running!"})
}

func USSDService(c *fiber.Ctx) error {
	type USSDData struct {
		Msisdn      string `form:"msisdn" validate:"required"`
		Input       string `form:"input" validate:"required"`
		SessionId   string `form:"session_id" validate:"required"`
		NetworkCode string `form:"network_code" validate:"required"`
		NewRequest  bool   `form:"new_request" validate:"required"`
	}
	ussd_data := USSDData{
		Msisdn:      c.Query("msisdn"),
		Input:       c.Query("input"),
		SessionId:   c.Query("session_id"),
		NetworkCode: c.Query("network_code"),
		NewRequest:  c.Query("new_request") == "1",
	}

	if err := Validate.Struct(ussd_data); err != nil {
		return utils.USSDResponse(c, ussd_data.NetworkCode, "FB", "Invalid request data, missing required fields")
	}
	message, err, isEndSession := processUSSD(&ussd_data.Input, ussd_data.Msisdn, ussd_data.SessionId, ussd_data.NetworkCode)
	if err != nil {
		if len(message) == 0 {
			message = systemError
		}
		utils.LogMessage("error", fmt.Sprintf("USSD error: %v", err), "ussd-service")
	}
	if isEndSession {
		return utils.USSDResponse(c, ussd_data.NetworkCode, "FB", message)
	}
	return utils.USSDResponse(c, ussd_data.NetworkCode, "FC", message)
}
func processUSSD(input *string, phone string, sessionId string, networkOperator string) (string, error, bool) {
	USSDdata, err := getUssdData(sessionId)
	// fmt.Println("USSDdata", USSDdata)
	if USSDdata != nil && USSDdata.StepId == "" {
		return "action_done", errors.New("no step id found, end session"), true
	}
	initialStep := "home"
	prefix := ""
	nextStep := ""
	resultMessage := ""
	isNewRequest := false
	customer := &model.Customer{}
	if USSDdata == nil || USSDdata.CustomerId == nil {
		// Re-fetch customer data
		err := config.DB.QueryRow(ctx, "select id,pgp_sym_decrypt(names::bytea,$1),network_operator,locale from customer where phone_hash = digest($2,'sha256')", encryptionKey, phone).
			Scan(&customer.Id, &customer.Names, &customer.NetworkOperator, &customer.Locale)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				initialStep = "welcome"
				// USSDdata.LastInput = *input
				// USSDdata.Id = sessionId
			} else {
				return "", err, true
			}
		} else {
			lang = customer.Locale
			// USSDdata.Id = sessionId
			// USSDdata.CustomerId = &customer.Id
			// USSDdata.CustomerName = customer.Names
			// USSDdata.Language = lang
		}
		if customer.NetworkOperator == "" {
			// Update network operator
			config.DB.Exec(ctx, "update customer set network_operator = $1 where id = $2", networkOperator, customer.Id)
		}
		if customer.Locale != "" {
			lang = customer.Locale
		}
	}
	if USSDdata == nil {
		isNewRequest = true
		//fetch initial step
		USSDdata = &model.USSDData{
			Id:           sessionId,
			MSISDN:       phone,
			CustomerId:   &customer.Id,
			CustomerName: customer.Names,
			Language:     lang,
			LastInput:    *input,
			StepId:       initialStep,
		}
		setUssdData(*USSDdata)
	}
	//load localizer
	localizer = loadLocalizer(lang)
	//get last step data
	lastStep := &model.USSDStep{}
	dataInputs := &[]model.USSDInput{}
	if USSDdata != nil && USSDdata.StepId != "" {
		stepData := viper.Get(fmt.Sprintf("steps.%s", USSDdata.StepId)).(map[string]interface{})
		inputs := stepData["inputs"].([]interface{})
		ussdInputs := make([]model.USSDInput, len(inputs))
		for i, input := range inputs {
			inpt := fmt.Sprintf("%v", input.(map[string]interface{})["input"])
			ac := fmt.Sprintf("%v", input.(map[string]interface{})["action"])
			nt := fmt.Sprintf("%v", input.(map[string]interface{})["next_step"])
			vl := fmt.Sprintf("%v", input.(map[string]interface{})["value"])
			vld := fmt.Sprintf("%v", input.(map[string]interface{})["validation"])
			ussdInputs[i] = model.USSDInput{
				Input:      inpt,
				Value:      vl,
				Action:     ac,
				NextStep:   nt,
				Validation: vld,
			}
		}
		vld2 := fmt.Sprintf("%v", stepData["validation"])
		lastStep = &model.USSDStep{
			Id:           stepData["id"].(string),
			Content:      stepData["content"].(string),
			AllowBack:    stepData["allow_back"].(bool),
			IsEndSession: stepData["is_end_session"].(bool),
			Validation:   &vld2,
			Inputs:       ussdInputs,
		}
		dataInputs = &lastStep.Inputs
	}
	if isNewRequest {
		setUssdData(*USSDdata)
		msg, err := prepareMessage(lastStep.Content, lang, input, phone, sessionId, customer, networkOperator)
		if err != nil {
			return "", err, false
		}

		if len(prefix) != 0 {
			msg = prefix + msg
		}
		return msg, nil, false
	}
	if nextData := USSDdata.NextData; nextData != "" && *input == "n" {
		// Display next data
		USSDdata.NextData = ""
		setUssdData(*USSDdata)
		msg, err := ellipsisMsg(nextData, sessionId, lang)
		return msg, err, false
	}

	if len(*dataInputs) != 0 {
		resItem, err := validateInputs(*dataInputs, input)
		if err != nil {
			return "", err, false
		}
		nextStep = resItem.NextStep
		//update next step
		USSDdata.NextStepId = nextStep
		USSDdata.StepId = nextStep
		// fmt.Println("next step 0: ", nextStep)
		setUssdData(*USSDdata)
		if validation := resItem.Validation; validation != "" {
			funcValue := reflect.ValueOf(map[string]interface{}{
				// Add your validation functions here
			}[validation])
			if funcValue.IsValid() {
				funcArgs := []reflect.Value{reflect.ValueOf(input), reflect.ValueOf(nextStep), reflect.ValueOf(false)}
				funcValue.Call(funcArgs)
			}
		}
		if action := resItem.Action; action != "" {
			resultMessage, err = callUserFunc(action, sessionId, lang, input, phone, customer, lang, USSDdata.LastInput, networkOperator)
			if err != nil {
				return "", err, true
			}
		}
	}
	// Handle next step and end session conditions
	if nextStep == "" && !lastStep.IsEndSession {
		// Log system bug
		utils.LogMessage("critical", fmt.Sprintf("No next step & is not end USSD: %v", USSDdata), "ussd-service")
		return "", errors.New("USSD system error"), true
	} else if lastStep.IsEndSession {
		if resultMessage == "" {
			return "Request successful", nil, true
		}
		return resultMessage, nil, false
	}
	// fmt.Println("nextStepData 0: ", viper.Get(fmt.Sprintf("steps.%v", nextStep)), fmt.Sprintf("steps.%v", nextStep))
	nextStepData := viper.Get(fmt.Sprintf("steps.%v", nextStep)).(map[string]interface{})
	// fmt.Println("nextStepData: ", nextStepData)
	if err != nil {
		// Log system bug
		utils.LogMessage("critical", fmt.Sprintf("Next step structure not found [%v]: %v", nextStep, USSDdata), "ussd-service")
		return "", errors.New("USSD system error"), true
	}

	msg, err := prepareMessage(nextStepData["content"].(string), lang, input, phone, sessionId, customer, networkOperator)
	if err != nil {
		return "", err, false
	}

	if len(prefix) != 0 {
		msg = prefix + msg
	}

	//save updated USSD data
	USSDdata.LastInput = *input
	USSDdata.LastResponse = msg
	// USSDdata.NextMenu = nextStepData["next_menu"].(string)
	// USSDdata.NextStepId = nextStepData["next_step_id"].(string)
	setUssdData(*USSDdata)
	if lastStep.IsEndSession {
		return msg, nil, true
	}
	return msg, nil, false
}
func ellipsisMsg(msg string, sessionId string, lang string) (string, error) {
	if msg != "" && utf8.RuneCountInString(msg) > USSD_MAX_LENGTH {
		nextMessage := "n." + lang // Adjust as needed for localization
		begin := msg[:USSD_MAX_LENGTH+1]
		pos := strings.LastIndex(begin, "\n")
		if pos != -1 {
			begin = msg[:pos+1]
			end := msg[pos+1:]
			msg = begin + "\n" + nextMessage

			mdl := &USSDFlow{}
			if err := mdl.SetNextData(sessionId, end); err != nil {
				return "", err
			}
		}
	}
	return msg, nil
}

// args: sessionId, lang, input, phone, customer, lang, USSDdata.LastInput, networkOperator
func callUserFunc(functionName string, args ...interface{}) (string, error) {
	funcValue := reflect.ValueOf(map[string]interface{}{
		// Add your functions here
		"savePreferredLang":       savePreferredLang,
		"preSavePreferredLang":    preSavePreferredLang,
		"preRegisterSaveCode":     preRegisterSaveCode,
		"preRegisterSaveName":     preRegisterSaveName,
		"getProvince":             getProvince,
		"preRegisterSaveProvince": preRegisterSaveProvince,
		"getDistrict":             getDistrict,
		"completeRegistration":    completeRegistration,
		"action_completed":        action_completed,
		"entrySaveCode":           entrySaveCode,
	}[functionName])
	if !funcValue.IsValid() {
		return "", fmt.Errorf("invalid input: %s", functionName)
	}
	funcArgs := make([]reflect.Value, len(args))
	for i, arg := range args {
		funcArgs[i] = reflect.ValueOf(arg)
	}
	result := funcValue.Call(funcArgs)
	if len(result) == 0 {
		return "", nil
	}

	msg, ok := result[0].Interface().(string)
	if !ok {
		return "", errors.New("function did not return a string")
	}

	return ellipsisMsg(msg, args[0].(string), args[1].(string))
}
func validateInputs(data []model.USSDInput, input *string) (*model.USSDInput, error) {
	optional := false
	itemRow := model.USSDInput{}
	for _, item := range data {
		// fmt.Println("input item: ", item.Input, item.Value)
		if item.Input != "" && input != nil && item.Input == *input {
			return &item, nil
		}
		if item.Input == "" {
			itemRow = item
			optional = true
		}
	}
	if optional {
		return &itemRow, nil
	}
	return nil, fmt.Errorf("invalid input: %s", *input)
}

func prepareMessage(data string, lang string, input *string, phone string, sessionId string, customer interface{}, operator interface{}) (string, error) {
	if strings.Contains(data, ":fn") {
		action := strings.Split(data, ":fn")[0]
		// fmt.Println("prepareMessage action: ", action, input)
		return callUserFunc(action, sessionId, lang, input, phone, customer, lang, operator)
	} else {
		msg := utils.Localize(localizer, data, nil)
		return ellipsisMsg(msg, sessionId, lang)
	}
}

func getUssdData(sessionId string) (*model.USSDData, error) {
	// get json ussd data from redis
	redisData, err := config.Redis.Get(ctx, "ussd:"+sessionId).Result()
	if err != nil {
		return nil, err
	}
	ussdData := model.USSDData{}
	err = json.Unmarshal([]byte(redisData), &ussdData)
	ussdData.Id = sessionId
	return &ussdData, err
}
func getUssdDataItem(sessionId string, itemKey string) (map[string]interface{}, error) {
	// get json ussd data from redis
	redisData, err := config.Redis.Get(ctx, "ussd:"+sessionId+"-"+itemKey).Result()
	if err != nil {
		return nil, err
	}
	ussdData := make(map[string]interface{})
	err = json.Unmarshal([]byte(redisData), &ussdData)
	return ussdData, err
}
func setUssdData(ussdData model.USSDData) error {
	// set json ussd data to redis
	jsonData, err := json.Marshal(ussdData)
	if err != nil {
		return err
	}
	fmt.Println("setUssdDataItem 1: ", string(jsonData), ussdData.Id)
	return config.Redis.Set(ctx, "ussd:"+ussdData.Id, jsonData, 120*time.Second).Err()
}
func setUssdDataItem(sessionId string, itemKey string, value map[string]interface{}) error {

	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return config.Redis.Set(ctx, "ussd:"+sessionId+"-"+itemKey, jsonData, 120*time.Second).Err()
}
func savePreferredLang(args ...interface{}) string {
	// Example function implementation
	fmt.Println("savePreferredLang called with args:", args)
	return "savePreferredLang "
}

// args: sessionId, lang, *input, phone, customer, lang, *USSDdata.LastInput, networkOperator
func preSavePreferredLang(args ...interface{}) string {
	input := args[2].(*string)
	sessionId := args[0].(string)
	extra, _ := getUssdDataItem(sessionId, "extra")
	lang := "en"
	if *input == "2" {
		lang = "rw"
	}
	appendExtraData(sessionId, extra, "preferred_lang", lang)
	return "saved"
}
func appendExtraData(sessionId string, extra map[string]interface{}, key string, value string) error {
	if len(extra) == 0 {
		extra = make(map[string]interface{})
	}
	extra[key] = value
	return setUssdDataItem(sessionId, "extra", extra)
}
func preRegisterSaveCode(args ...interface{}) string {
	input := args[2].(*string)
	sessionId := args[0].(string)
	extra, _ := getUssdDataItem(sessionId, "extra")
	appendExtraData(sessionId, extra, "code", *input)
	return "saved"
}
func getProvince(args ...interface{}) string {
	// Example function implementation
	return "getProvince "
}
func preRegisterSaveProvince(args ...interface{}) string {
	// Example function implementation
	return "preRegisterSaveProvince "
}
func getDistrict(args ...interface{}) string {
	// Example function implementation
	return "getDistrict "
}
func completeRegistration(args ...interface{}) string {
	// Example function implementation
	return "completeRegistration "
}
func preRegisterSaveName(args ...interface{}) string {
	// Example function implementation
	input := args[2].(*string)
	sessionId := args[0].(string)
	extra, _ := getUssdDataItem(sessionId, "extra")
	appendExtraData(sessionId, extra, "name", *input)
	return "saved"
}
func action_completed(args ...interface{}) string {
	// Example function implementation
	return "action_completed "
}
func entrySaveCode(args ...interface{}) string {
	// Example function implementation
	return "entrySaveCode "
}
