package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"shared-package/utils"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"ussd-service/config"
	"ussd-service/model"

	"math/rand"

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
		fmt.Println("Error loading EN translations:", err)
	}
	_, err = bundle.LoadMessageFile("/app/locales/ussd.sw.toml")
	if err != nil {
		fmt.Println("Error loading translations:", err)
	}
}
func loadLocalizer(lang string) *i18n.Localizer {
	if lang == "rw" {
		return i18n.NewLocalizer(bundle, "sw")
	}
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
		SessionId   string `form:"sessionId" validate:"required"`
		NetworkCode string `form:"networkCode" validate:"required"`
		NewRequest  bool   `form:"newRequest"`
	}
	ussd_data := USSDData{
		Msisdn:      c.Query("msisdn"),
		Input:       c.Query("input"),
		SessionId:   c.Query("sessionId"),
		NetworkCode: c.Query("networkCode"),
		NewRequest:  c.Query("newRequest") == "1",
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

var USSDdata *model.USSDData

func processUSSD(input *string, phone string, sessionId string, networkOperator string) (string, error, bool) {
	USSDdata, _ = getUssdData(sessionId)
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
		err := config.DB.QueryRow(ctx, "select id,pgp_sym_decrypt(names::bytea,$1),network_operator,locale from customer where phone_hash = digest($2,'sha256')", config.EncryptionKey, phone).
			Scan(&customer.Id, &customer.Names, &customer.NetworkOperator, &customer.Locale)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				initialStep = "welcome"
				// USSDdata.LastInput = *input
				// USSDdata.Id = sessionId
			} else {
				return "", fmt.Errorf("fetch customer failed, err: %v", err), true
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
	if isNewRequest && USSDdata.StepId == "welcome" {
		//check if phone number is valid
		names, err := utils.ValidateMTNPhone(phone)
		if err != nil {
			//load localizer
			localizer = loadLocalizer(lang)
			msg := utils.Localize(localizer, err.Error(), nil)
			return msg, nil, true
		}
		extraData := make(map[string]interface{})
		appendExtraData(sessionId, extraData, "momo_names", names)
		appendExtraData(sessionId, extraData, "name", names)
	}
	if !isNewRequest && USSDdata.StepId == "welcome" {
		if *input == "1" {
			USSDdata.Language = "en"
		} else {
			USSDdata.Language = "rw"
		}
		lang = USSDdata.Language
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
		// setUssdData(*USSDdata)
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
			if action == "end_session" {
				msg := utils.Localize(localizer, "thank_you", nil)
				return msg, nil, true
			}
			resultMessage, err = callUserFunc(action, sessionId, lang, input, phone, customer, lang, USSDdata.LastInput, networkOperator)
			if err != nil {
				if len(resultMessage) == 0 {
					return resultMessage, err, true
				}
				return resultMessage, err, false
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
	if USSDdata.StepId == "action_ack" {
		lang = USSDdata.Language
		localizer = loadLocalizer(lang)
	}
	//load localizer
	// fmt.Println("nextStepData: ", nextStepData)
	// if err != nil {
	// 	// Log system bug
	// 	utils.LogMessage("critical", fmt.Sprintf("Next step structure not found [%v]: %v", nextStep, USSDdata), "ussd-service")
	// 	return "", errors.New("USSD system error"), true
	// }

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
		"end_session":             end_session,
	}[functionName])
	if !funcValue.IsValid() {
		return "", fmt.Errorf("invalid function call: %s, arg: %v", functionName, args)
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
	if strings.Contains(msg, "err:") {
		return "", errors.New(strings.Split(msg, "err:")[1])
	} else if strings.Contains(msg, "fail:") {
		failMsg := utils.Localize(localizer, strings.Split(msg, "fail:")[1], nil)
		return failMsg, errors.New(failMsg)
	}
	if len(msg) != 0 {
		msg = utils.Localize(localizer, msg, nil)
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
	return nil, fmt.Errorf("invalid input : %s", *input)
}

func prepareMessage(data string, lang string, input *string, phone string, sessionId string, customer interface{}, operator interface{}) (string, error) {
	if strings.Contains(data, ":fn") {
		action := strings.Split(data, ":fn")[0]
		// fmt.Println("prepareMessage action: ", action, input)
		return callUserFunc(action, sessionId, lang, input, phone, customer, lang, operator)
	} else {
		var arg map[string]interface{} = nil
		if data == "home_ussd" {
			arg = map[string]interface{}{"Name": customer.(*model.Customer).Names}
		}
		msg := utils.Localize(localizer, data, arg)
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
func getUssdDataItem(sessionId string, itemKey string) (interface{}, error) {
	// get json ussd data from redis
	redisData, err := config.Redis.Get(ctx, "ussd:"+sessionId+"-"+itemKey).Result()
	if err != nil {
		return nil, err
	}
	if itemKey == "data" {
		ussdData := []map[string]interface{}{}
		err = json.Unmarshal([]byte(redisData), &ussdData)
		return ussdData, err
	} else {
		ussdData := make(map[string]interface{})
		err = json.Unmarshal([]byte(redisData), &ussdData)
		return ussdData, err
	}
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
func setUssdDataItem(sessionId string, itemKey string, value interface{}) error {

	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return config.Redis.Set(ctx, "ussd:"+sessionId+"-"+itemKey, jsonData, 120*time.Second).Err()
}
func savePreferredLang(args ...interface{}) string {
	input := args[2].(*string)
	lang := "en"
	if *input == "2" {
		lang = "rw"
	}
	_, err := config.DB.Exec(ctx, "update customer set locale = $1 where id = $2", lang, USSDdata.CustomerId)
	if err != nil {
		utils.LogMessage("error", "savePreferredLang: update customer failed: err:"+err.Error(), "ussd-service")
		return "err:system_error"
	}

	//update USSD data
	USSDdata.Language = lang
	return ""
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
	if extra == nil || reflect.ValueOf(extra).IsNil() {
		extra = make(map[string]interface{})
	}
	extraData := extra.(map[string]interface{})
	appendExtraData(sessionId, extraData, "preferred_lang", lang)
	return ""
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
	//validate code
	code := strings.ToUpper(*input)
	var codeId int
	var status string
	err := config.DB.QueryRow(ctx, `select id,status from codes where code_hash = digest($1,'sha256')`, code).Scan(&codeId, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "fail:invalid_code"
		}
		utils.LogMessage("error", "preRegisterSaveCode: fetch code id failed: err:"+err.Error(), "ussd-service")
		return "fail:system_error"
	}
	if status != "unused" {
		return "fail:inactive_code"
	}
	extra, _ := getUssdDataItem(sessionId, "extra")
	extraData := extra.(map[string]interface{})
	appendExtraData(sessionId, extraData, "code", strings.ToUpper(*input))
	appendExtraData(sessionId, extraData, "code_id", fmt.Sprintf("%v", codeId))
	return ""
}
func getProvince(args ...interface{}) string {
	sessionId := args[0].(string)
	//fetch all provinces from db
	rows, err := config.DB.Query(ctx, "select id,name from province")
	if err != nil {
		utils.LogMessage("error", "getProvince: fetch rows failed: err:"+err.Error(), "ussd-service")
		return "System error. Please try again later."
	}
	defer rows.Close()
	provinces := []model.Province{}
	provincesText := ""
	a := 1
	for rows.Next() {
		province := model.Province{}
		err := rows.Scan(&province.Id, &province.Name)
		if err != nil {
			utils.LogMessage("error", "getProvince: scan row failed: err:"+err.Error(), "ussd-service")
			return "System error. Please try again later."
		}
		provinces = append(provinces, province)
		provincesText += fmt.Sprintf("%v) %s\n", a, province.Name)
		a++
	}
	setUssdDataItem(sessionId, "data", provinces)
	return utils.Localize(localizer, "select_province", map[string]interface{}{"Provinces": provincesText})
}
func preRegisterSaveProvince(args ...interface{}) string {
	input := args[2].(*string)
	sessionId := args[0].(string)
	data, _ := getUssdDataItem(sessionId, "data")
	provinces := data.([]map[string]interface{})
	//check if input is a number
	inputId := 0
	if num, err := strconv.Atoi(*input); err == nil {
		inputId = num
	} else {
		return "err:input_must_number"
	}
	province := provinces[(inputId - 1)]
	fmt.Println("selected province: ", province)
	extra, _ := getUssdDataItem(sessionId, "extra")
	extraData := extra.(map[string]interface{})
	appendExtraData(sessionId, extraData, "province", fmt.Sprintf("%v", province["Id"]))
	return ""
}
func getDistrict(args ...interface{}) string {
	sessionId := args[0].(string)
	extra, _ := getUssdDataItem(sessionId, "extra")
	extraData := extra.(map[string]interface{})
	provinceId, ok := extraData["province"]
	if !ok {
		return "err:province_not_selected"
	}
	//fetch all provinces from db
	rows, err := config.DB.Query(ctx, "select id,name from district where province_id=$1", provinceId)
	if err != nil {
		utils.LogMessage("error", "getDistrict: fetch rows failed: err:"+err.Error(), "ussd-service")
		return "System error. Please try again later."
	}
	defer rows.Close()
	districts := []model.District{}
	districtText := ""
	a := 1
	for rows.Next() {
		district := model.District{}
		err := rows.Scan(&district.Id, &district.Name)
		if err != nil {
			utils.LogMessage("error", "getDistrict: scan row failed: err:"+err.Error(), "ussd-service")
			return "System error. Please try again later."
		}
		districts = append(districts, district)
		districtText += fmt.Sprintf("%v) %s\n", a, district.Name)
		a++
	}
	setUssdDataItem(sessionId, "data", districts)
	return utils.Localize(localizer, "select_district", map[string]interface{}{"Districts": districtText})
}
func action_completed(args ...interface{}) string {
	return "success_entry"
}
func preRegisterSaveName(args ...interface{}) string {
	// Example function implementation
	input := args[2].(*string)
	sessionId := args[0].(string)
	extra, _ := getUssdDataItem(sessionId, "extra")
	extraData := extra.(map[string]interface{})
	appendExtraData(sessionId, extraData, "name", *input)
	return ""
}

// get district and save customer
func completeRegistration(args ...interface{}) string {
	input := args[2].(*string)
	sessionId := args[0].(string)
	data, _ := getUssdDataItem(sessionId, "data")
	districts := data.([]map[string]interface{})
	//check if input is a number
	inputId := 0
	if num, err := strconv.Atoi(*input); err == nil {
		inputId = num
	} else {
		return "err:input_must_number"
	}
	district := districts[(inputId - 1)]
	fmt.Println("selected district: ", district)
	extra, _ := getUssdDataItem(sessionId, "extra")
	extraData := extra.(map[string]interface{})
	provinceId := extraData["province"]
	// name := extraData["name"]
	name := extraData["name"]
	momo_names := extraData["momo_names"]
	var customerId int
	err := config.DB.QueryRow(ctx, `insert into customer (names,momo_names,phone,phone_hash,province,district,locale, network_operator) values
	(pgp_sym_encrypt($1,$2),pgp_sym_encrypt($8,$2),pgp_sym_encrypt($3,$2)::bytea,digest($3,'sha256')::bytea,$4,$5,$6,$7) returning id`,
		name, config.EncryptionKey, args[3].(string), provinceId, district["Id"], extraData["preferred_lang"], args[7].(string), momo_names).Scan(&customerId)
	if err != nil {
		utils.LogMessage("error", "completeRegistration: insert customer failed: err:"+err.Error(), "ussd-service")
		return "err:system_error"
	}
	var entryId int
	//create entry record
	err = config.DB.QueryRow(ctx, `insert into entries (customer_id,code_id) values ($1,$2) returning id`, customerId, extraData["code_id"]).Scan(&entryId)
	if err != nil {
		//delete customer
		removeCustomer(customerId)
		utils.LogMessage("error", "completeRegistration: insert entries failed: err:"+err.Error(), "ussd-service")
		return "err:system_error"
	}
	//create entry record
	_, err = config.DB.Exec(ctx, `update codes set status = 'used' where id = $1`, extraData["code_id"])
	if err != nil {
		//delete customer
		removeCustomer(customerId)
		//delete entry
		_, err = config.DB.Exec(ctx, `delete from entries where customer_id = $1 and code_id=$2`, customerId, extraData["code_id"])
		utils.LogMessage("error", "completeRegistration: insert entry failed: err:"+err.Error(), "ussd-service")
		return "err:system_error"
	}
	USSDdata.CustomerId = &customerId
	sms_message, message_type, _, err := dailyPrizeWinning(entryId, extraData["code"].(string), args[1].(string))
	if err != nil {
		return err.Error()
	}
	go utils.SendSMS(config.DB, args[3].(string), sms_message, viper.GetString("SENDER_ID"), config.ServiceName, message_type, &customerId, config.Redis)
	return "success_entry"
}
func removeCustomer(customerId int) {
	_, err := config.DB.Exec(ctx, `delete from customer where id = $1`, customerId)
	if err != nil {
		utils.LogMessage("error", "removeCustomer: delete customer failed: err:"+err.Error(), "ussd-service")
	}
}
func entrySaveCode(args ...interface{}) string {
	code := strings.ToUpper(*args[2].(*string))
	var codeId int
	var status string
	err := config.DB.QueryRow(ctx, `select id,status from codes where code_hash = digest($1,'sha256')`, code).Scan(&codeId, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "fail:invalid_code"
		}
		utils.LogMessage("error", "preRegisterSaveCode: fetch code id failed: err:"+err.Error(), "ussd-service")
		return "fail:system_error"
	}
	if status != "unused" {
		return "fail:inactive_code"
	}
	var entryId int
	//create entry record
	err = config.DB.QueryRow(ctx, `insert into entries (customer_id,code_id) values ($1,$2) returning id`, USSDdata.CustomerId, codeId).Scan(&entryId)
	if err != nil {
		//delete customerY
		utils.LogMessage("error", "entrySaveCode: insert entries failed: err:"+err.Error(), "ussd-service")
		return "err:system_error"
	}
	//create entry record
	_, err = config.DB.Exec(ctx, `update codes set status = 'used' where id = $1`, codeId)
	if err != nil {
		//delete entry
		_, err = config.DB.Exec(ctx, `delete from entries where customer_id = $1 and code_id=$2`, USSDdata.CustomerId, codeId)
		utils.LogMessage("error", "entrySaveCode: insert entry failed: err:"+err.Error(), "ussd-service")
		return "err:system_error"
	}
	sms_message, message_type, _, err := dailyPrizeWinning(entryId, code, args[1].(string))
	if err != nil {
		return err.Error()
	}
	go utils.SendSMS(config.DB, args[3].(string), sms_message, viper.GetString("SENDER_ID"), config.ServiceName, message_type, USSDdata.CustomerId, config.Redis)
	return ""
}
func end_session(args ...interface{}) string {
	return "success_entry"
}

func dailyPrizeWinning(entryId int, code string, lang string) (string, string, bool, error) {
	// Create a new rand instance with a secure seed
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	result := utils.GenerateBoolWithOdds(rng)
	var prizeType struct {
		Id             int
		Name           string
		RemainingPlace int
		Value          int
		Status         string
		DistrutionType string
		Message        string
	}
	var isPrizeWon bool
	var prizeId int
	if result {
		//try to get daily prize and check if there is a remaining room (based on elligibility and distributed prizes)
		//get daily prize typey
		err := config.DB.QueryRow(ctx, `select pt.id, pt.name,(pt.elligibility - count(p.id)) as remaining_place,pt.value,pt.status,pt.distribution_type from prize_type pt
		LEFT JOIN prize p on p.prize_type_id = pt.id where pt.period = 'DAILY' and pt.trigger_by_system = true and pt.status='OKAY' group by pt.id, p.prize_type_id order by random() limit 1`).
			Scan(&prizeType.Id, &prizeType.Name, &prizeType.RemainingPlace, &prizeType.Value, &prizeType.Status, &prizeType.DistrutionType)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				isPrizeWon = false
			}
			utils.LogMessage("error", "entrySaveCode: fetch daily prize failed: err:"+err.Error(), "ussd-service")
			return "", "", false, errors.New("err:system_error")
		}
		if prizeType.RemainingPlace > 0 {
			//fetch prize_message
			err = config.DB.QueryRow(ctx, `select message from prize_message where prize_type_id = $1 and lang = $2`, prizeType.Id, lang).Scan(&prizeType.Message)
			if err != nil {
				utils.LogMessage("error", "entrySaveCode: fetch prize message failed: err:"+err.Error(), "ussd-service")
				return "", "", false, errors.New("err:system_error")
			}
			//create prize record
			err = config.DB.QueryRow(ctx, `insert into prize (entry_id, prize_type_id, prize_value,code,rewarded) values ($1, $2, $3,$4, false) returning id`,
				entryId, prizeType.Id, prizeType.Value, code).Scan(&prizeId)
			if err != nil {
				utils.LogMessage("error", "entrySaveCode: insert prize failed: err:"+err.Error(), "ussd-service")
				return "", "", false, errors.New("err:system_error")
			}
			isPrizeWon = true
		}

	}
	var sms_message, message_type string
	if isPrizeWon {
		sms_message = prizeType.Message
		message_type = "prize_won"
		if prizeType.DistrutionType == "momo" {
			//fetch	customer phone and network operator
			var mno string
			err := config.DB.QueryRow(ctx, `select network_operator from customer where id = $1`,
				USSDdata.CustomerId).Scan(&mno)
			if err != nil {
				utils.LogMessage("error", "entrySaveCode: #distribute_prize fetch customer MNO failed: err:"+err.Error(), "ussd-service")
			} else {
				_, err = config.DB.Exec(ctx, `insert into transaction (prize_id, amount, phone, mno, customer_id, transaction_type, initiated_by,status) values ($1, $2, $3, $4, $5,'DEBIT','SYSTEM','PENDING')`,
					prizeId, prizeType.Value, USSDdata.MSISDN, mno, USSDdata.CustomerId)
				if err != nil {
					utils.LogMessage("error", "entrySaveCode: #distribute_prize insert transaction failed: err:"+err.Error(), "ussd-service")
				}
			}
		}
	} else {
		sms_message = utils.Localize(localizer, "register_sms", nil)
		message_type = "no_prize"
	}
	return sms_message, message_type, isPrizeWon, nil
}
