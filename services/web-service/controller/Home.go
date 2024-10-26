package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"logger-service/helper"
	"os"
	"strconv"
	"strings"
	"time"
	"web-service/config"
	"web-service/model"

	"shared-package/utils"

	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

var Validate = validator.New()
var ctx = context.Background()

func init() {
	// Register the custom validation function
	err := Validate.RegisterValidation("regex", utils.RegexValidation)
	if err != nil {
		utils.LogMessage("critical", "Init: Error registering regex validation", config.ServiceName)
		panic("Init: Error registering regex validation")
	}
}

/*
Receive deleteCache request
*/
func Index(c *fiber.Ctx) error {
	//helper.SecurePath(c)
	c.Accepts("text/plain", "application/json")
	return c.JSON(fiber.Map{"status": 200,
		"message": "Weclome to go Fiber microservice kickstart project from Qonics inc",
	})
}

func ServiceStatusCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": 200, "message": "This API service is running!"})
}
func GetSVGAvatar(c *fiber.Ctx) error {
	typee := c.Params("type", "av")
	avatarNumber, err := strconv.Atoi(c.Params("avatar_number", "1"))
	if err != nil || avatarNumber > 89 {
		c.SendStatus(fiber.StatusForbidden)
		return c.SendString("Please provide a valid avatar info")
	}
	location := "corporate"
	if typee == "av" {
		location = "avatars"
	}
	filePath := fmt.Sprintf("/app/assets/svg/%s/avatar_%d.svg", location, avatarNumber)
	file, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.SendStatus(fiber.StatusNotFound)
			//provide default avatar
			file, _ = os.ReadFile(fmt.Sprintf("/app/assets/svg/%s/avatar_1.svg", location))
		} else {
			c.SendStatus(fiber.StatusForbidden)
			fmt.Println("Unable to get avatar, err: " + err.Error())
			return c.SendString("We have an issue on our end!")
		}
	}
	// Set the Cache-Control header to cache the image for one year
	c.Set("Cache-Control", "public, max-age=31536000")
	c.Set("Content-Type", "image/svg+xml")
	return c.SendString(string(file))
}
func LoginWithEmail(c *fiber.Ctx) error {
	type UserData struct {
		Email    string `json:"email" binding:"required" validate:"required,email"`
		Password string `json:"password" validate:"required"`
	}
	responseStatus := 200
	userData := new(UserData)
	if err := c.BodyParser(userData); err != nil || userData.Email == "" {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data", "details": err})
	}
	if err := Validate.Struct(userData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Provide data are not valid")
	}
	invalidKeys := utils.ValidateStruct(userData, []string{}, []string{"Password"})
	errorMessage := utils.ValidateStructText(invalidKeys)
	if errorMessage != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, *errorMessage)
	}
	userData.Email = strings.ToLower(userData.Email)
	//check user data
	UserProfile := model.UserProfile{}
	err := config.DB.QueryRow(ctx,
		`select u.id,u.fname,u.lname,u.department_id,d.title as department_title, u.email_verified,u.phone_verified,u.avatar_url,u.status,
	u.can_add_codes,u.can_trigger_draw,u.can_add_user,u.can_view_logs,phone from users u inner join departments d on u.department_id = d.id where email = $1 and password = crypt($2, password)`, userData.Email, userData.Password).
		Scan(&UserProfile.Id, &UserProfile.Fname, &UserProfile.Lname, &UserProfile.Department.Id, &UserProfile.Department.Title, &UserProfile.EmailVerified, &UserProfile.PhoneVerified, &UserProfile.AvatarUrl, &UserProfile.Status,
			&UserProfile.CanAddCodes, &UserProfile.CanTriggerDraw, &UserProfile.CanAddUser, &UserProfile.CanViewLogs, &UserProfile.Phone)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			utils.LogMessage("critical", fmt.Sprintf("LoginWithEmail: Unable to get user data, Email:%s, err:%v", userData.Email, err), "web-service")
		}
		responseStatus = 403
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Invalid credentials"})
	} else if UserProfile.Status == "inactive" {
		responseStatus = 403
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Your account has been deactivated"})
	}
	UserProfile.Email = userData.Email
	//Generate and save access token
	token, err := generateAccesstoken(UserProfile)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Login failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	c.SendStatus(responseStatus)
	return c.JSON(fiber.Map{"status": responseStatus, "message": "Login completed", "data": UserProfile, "accessToken": token})
}

func generateAccesstoken(userData model.UserProfile) (string, error) {
	payloadData, err := json.Marshal(userData)
	if err != nil {
		return "", fmt.Errorf("unable to marshal payload data for user %d , error: %s", userData.Id, err.Error())
	}
	token := base64.RawStdEncoding.EncodeToString([]byte(fmt.Sprintf("token_%v_%v", userData.Id, time.Now().UnixMilli())))
	if err := config.Redis.Set(ctx, token, payloadData, time.Duration(helper.SessionExpirationTime*time.Minute)).Err(); err != nil {
		return "", fmt.Errorf("unable to save user access token for user %d , error: %s", userData.Id, err.Error())
	}
	return token, nil
}
func GetUserProfile(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	UserProfile := model.UserProfile{}
	err = config.DB.QueryRow(ctx,
		`select u.id,u.fname,u.lname,u.department_id,d.title as department_title, u.email_verified,u.phone_verified,u.avatar_url,u.status,
	u.can_add_codes,u.can_trigger_draw,u.can_add_user,u.can_view_logs from users u inner join departments d on u.department_id = d.id where u.id = $1`, userPayload.Id).
		Scan(&UserProfile.Id, &UserProfile.Fname, &UserProfile.Lname, &UserProfile.Department.Id, &UserProfile.Department.Title, &UserProfile.EmailVerified, &UserProfile.PhoneVerified, &UserProfile.AvatarUrl, &UserProfile.Status,
			&UserProfile.CanAddCodes, &UserProfile.CanTriggerDraw, &UserProfile.CanAddUser, &UserProfile.CanViewLogs)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get user profile failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetUserProfile: Unable to verify user info, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "User data is not valid")
	} else if UserProfile.Status != "OKAY" {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Your account is not active")
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": UserProfile})
}

func GetPrizeCategory(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	categories := []model.PrizeCategory{}
	rows, err := config.DB.Query(ctx,
		`select id,name,status,created_at from prize_category`)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get category data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeCategory: Unable to get category data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "category data is not valid")
	}
	for rows.Next() {
		category := model.PrizeCategory{}
		err = rows.Scan(&category.Id, &category.Name, &category.Status, &category.CreatedAt)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get category data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeCategory: Unable to get category data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		categories = append(categories, category)
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": categories})
}
func GetPrizeType(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	prizeCategory := c.Params("prize_category")
	prizes := []model.PrizeType{}
	var rows pgx.Rows
	if prizeCategory == "" {
		rows, err = config.DB.Query(ctx,
			`select p.id,p.name,p.status,p.value,p.elligibility,pc.name as category_name,pc.id as category_id,pc.status as category_status,pc.created_at,p.created_at,
			p.period,p.distribution_type,p.expiry_date,STRING_AGG(pm.lang, ', ') as langs,STRING_AGG(pm.message, ', ') as messages from prize_type p join prize_category pc on p.prize_category_id = pc.id join prize_message pm on pm.prize_type_id=p.id group by p.id,pc.id`)
	} else {
		rows, err = config.DB.Query(ctx,
			`select p.id,p.name,p.status,p.value,p.elligibility,pc.name as category_name,pc.id as category_id,pc.status as category_status,pc.created_at,p.created_at,
			p.period,p.distribution_type,p.expiry_date,STRING_AGG(pm.lang, ', ') as langs,STRING_AGG(pm.message, ', ') as messages from prize_type p join prize_category pc on p.prize_category_id = pc.id join prize_message pm on pm.prize_type_id=p.id where p.prize_category_id=$1 group by p.id,pc.id`, prizeCategory)
	}
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prize type data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeType: Unable to get prize type data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "prize type data is not valid")
	}
	for rows.Next() {
		var langs, messages string
		prize := model.PrizeType{}
		err = rows.Scan(&prize.Id, &prize.Name, &prize.Status, &prize.Value, &prize.Elligibility, &prize.PrizeCategory.Name,
			&prize.PrizeCategory.Id, &prize.PrizeCategory.Status, &prize.PrizeCategory.CreatedAt, &prize.CreatedAt, &prize.Period, &prize.Distribution, &prize.ExpiryDate, &langs, &messages)
		//extract messages and langs and populate to []prize.Message
		prize.PrizeMessage = []model.PrizeMessage{}
		for i, lang := range strings.Split(langs, ", ") {
			prize.PrizeMessage = append(prize.PrizeMessage, model.PrizeMessage{Lang: lang, Message: strings.Split(messages, ", ")[i]})
		}
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prize type data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeType: Unable to get prize type data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		prizes = append(prizes, prize)
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": prizes})
}
func GetEntries(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	entries := []model.Entries{}
	rows, err := config.DB.Query(ctx,
		`select e.id,e.code_id,e.customer_id,e.created_at,p.id as province_id,p.name as province_name,d.id as district_id,d.name as district_name,
		c.created_at,pt.name as prize_type_name,pt.id as prize_type_id,pt.value as prize_type_value,cd.created_at,c.network_operator,c.locale from entries e
		inner join customer c on e.customer_id = c.id
		inner join codes cd on e.code_id = cd.id
		inner join province p on c.province = p.id
		inner join district d on c.district = d.id
		LEFT JOIN prize_type pt on cd.prize_type_id = pt.id`)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get entries data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetEntries: Unable to get entries data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "entries data is not valid")
	}
	for rows.Next() {
		entry := model.Entries{}
		var prizeTypeName *string
		var prizeTypeId, prizeTypeValue *int
		err = rows.Scan(&entry.Id, &entry.Code.Id, &entry.Customer.Id, &entry.CreatedAt, &entry.Customer.Province.Id, &entry.Customer.Province.Name,
			&entry.Customer.District.Id, &entry.Customer.District.Name, &entry.Customer.CreatedAt, &prizeTypeName, &prizeTypeId, &prizeTypeValue,
			&entry.Code.CreatedAt, &entry.Customer.NetworkOperator, &entry.Customer.Locale)
		entry.Customer.Phone = "**********"
		entry.Customer.Names = "**********"
		entry.Code.Code = "**********"
		if prizeTypeName != nil {
			entry.Code.PrizeType = &model.PrizeType{}
			entry.Code.PrizeType.Name = *prizeTypeName
			entry.Code.PrizeType.Id = *prizeTypeId
			entry.Code.PrizeType.Value = *prizeTypeValue
		}
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get entries data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetEntries: Unable to get entries data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		entries = append(entries, entry)
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": entries})
}

func GetPrizes(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	prizes := []model.Prize{}
	rows, err := config.DB.Query(ctx,
		`select p.id,p.rewarded,p.created_at,p.prize_value,p.prize_type_id,pc.name as category_name,pc.status as category_status,pc.created_at as category_created_at,
		e.customer_id from prize p
		inner join entries e on p.entry_id = e.id
		inner join prize_type pt on pt.id = p.prize_type_id
		inner join prize_category pc on pt.prize_category_id = pc.id`)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prizes data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizes: Unable to get entries data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "prizes data is not valid")
	}
	for rows.Next() {
		prize := model.Prize{}
		err = rows.Scan(&prize.Id, &prize.Rewarded, &prize.CreatedAt, &prize.Value, &prize.PrizeType.Id, &prize.PrizeCategory.Name, &prize.PrizeCategory.Status, &prize.PrizeCategory.CreatedAt, &prize.Customer.Id)
		prize.Customer.Phone = "**********"
		prize.Customer.Names = "**********"
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prizes data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizes: Unable to get prizes data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		prizes = append(prizes, prize)
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": prizes})
}

func CreatePrizeCategory(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type FormData struct {
		Name string `json:"name" binding:"required" validate:"required,regex=^[a-zA-Z0-9\\-_ ]*$"`
	}
	responseStatus := 200
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil || formData.Name == "" {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data - " + formData.Name, "details": err})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provide data are not valid")
	}
	invalidKeys := utils.ValidateStruct(formData, []string{}, []string{})
	errorMessage := utils.ValidateStructText(invalidKeys)
	if errorMessage != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, *errorMessage)
	}
	_, err = config.DB.Exec(ctx,
		`insert into prize_category (name,status,operator_id) values ($1,'OKAY',$2)`, formData.Name, userPayload.Id)
	if err != nil {
		if ok, key := utils.IsErrDuplicate(err); ok {
			return utils.JsonErrorResponse(c, fiber.StatusConflict, fmt.Sprintf("Unable to save data, %s already exists", key))
		}
		responseStatus = fiber.StatusConflict
		c.SendStatus(responseStatus)
		return utils.JsonErrorResponse(c, fiber.StatusConflict, "Unable to save data, system error. please try again later", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("CreatePrizeCategory: Unable to save data, Name:%s, err:%v", formData.Name, err),
			ServiceName: config.ServiceName,
		})
	}
	return c.JSON(fiber.Map{"status": responseStatus, "message": "Prize category added successfully"})
}
func CreatePrizeType(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}

	type FormData struct {
		Name         string               `json:"name" binding:"required" validate:"required,regex=^[a-zA-Z0-9\\-_ #@]*$"`
		CategoryId   int                  `json:"category_id" binding:"required" validate:"required,number"`
		Value        int                  `json:"value" binding:"required" validate:"required,number"`
		Elligibility int                  `json:"elligibility" binding:"required" validate:"required,number"`
		ExpiryDate   string               `json:"expiry_date" binding:"required" validate:"required"`
		Period       string               `json:"period" binding:"required" validate:"required,oneof=monthly weekly daily"`
		Distribution string               `json:"distribution" binding:"required" validate:"required,oneof=momo cheque other"`
		Messages     []model.PrizeMessage `json:"messages" binding:"required" validate:"required"`
	}
	responseStatus := 200
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil || formData.Name == "" {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data", "details": err})
	}
	if err := Validate.Struct(formData); err != nil {
		c.SendStatus(fiber.StatusNotAcceptable)
		return c.JSON(fiber.Map{"status": fiber.StatusNotAcceptable, "message": "Provide data are not valid", "details": err})
	}
	invalidKeys := utils.ValidateStruct(formData, []string{"#", "@"}, []string{"ExpiryDate"})
	errorMessage := utils.ValidateStructText(invalidKeys)
	if errorMessage != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, *errorMessage)
	}
	//check if expiry_date is already expired
	expiryDate, err := time.Parse("02/01/2006", formData.ExpiryDate)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provide data are not valid")
	}
	if expiryDate.Before(time.Now()) {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Expiry date is already expired")
	}
	tx, err := config.DB.Begin(ctx)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save data, system error. please try again later", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("CreatePrizeType: Unable to begin transaction, Name:%s, err:%v", formData.Name, err),
			ServiceName: config.ServiceName,
		})
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(context.Background()); rbErr != nil {
				utils.LogMessage("critical", fmt.Sprintf("CreatePrizeType: Unable to rollback transaction, Name:%s, err:%v", formData.Name, rbErr), config.ServiceName)
			}
		}
	}()
	var prizeTypeId int
	err = tx.QueryRow(ctx,
		`insert into prize_type (name,prize_category_id,elligibility,value,status,operator_id,expiry_date,distribution_type,period)
		values ($1,$2,$3,$4,'OKAY',$5, $6, $7, $8)  returning id`,
		formData.Name, formData.CategoryId, formData.Elligibility, formData.Value, userPayload.Id, expiryDate, formData.Distribution, formData.Period).
		Scan(&prizeTypeId)
	if err != nil {
		if ok, key := utils.IsErrDuplicate(err); ok {
			return utils.JsonErrorResponse(c, fiber.StatusConflict, fmt.Sprintf("Unable to save data, %s already exists", key))
		} else if ok, key := utils.IsForeignKeyErr(err); ok {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, fmt.Sprintf("Unable to save data, %s is invalid", key))
		}
		responseStatus = fiber.StatusConflict
		c.SendStatus(responseStatus)
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save data, system error. please try again later", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("CreatePrizeType: Unable to save data, Name:%s, err:%v", formData.Name, err),
			ServiceName: config.ServiceName,
		})
	}
	//save message
	for _, message := range formData.Messages {
		_, err = tx.Exec(ctx,
			`insert into prize_message (lang, prize_type_id, message, operator_id) values ($1, $2, $3, $4)`, message.Lang, prizeTypeId, message.Message, userPayload.Id)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save data, system error. please try again later", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     fmt.Sprintf("CreatePrizeType: Unable to save message, Name:%s, err:%v", formData.Name, err),
				ServiceName: config.ServiceName,
			})
		}
	}
	if err = tx.Commit(context.Background()); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save data, system error. please try again later", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("CreatePrizeType: Unable to commit transaction, Name:%s, err:%v", formData.Name, err),
			ServiceName: config.ServiceName,
		})
	}
	return c.JSON(fiber.Map{"status": responseStatus, "message": "Prize type added successfully"})
}
func GetDraws(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	draws := []model.Draw{}
	rows, err := config.DB.Query(ctx,
		`select d.id,d.code,d.customer_id,d.created_at,p.id as province_id,p.name as province_name,ds.id as district_id,ds.name as district_name,
		c.created_at,pt.name as prize_type_name,pt.id as prize_type_id,pt.value as prize_type_value,c.network_operator,c.locale from draw d
		inner join customer c on d.customer_id = c.id
		inner join province p on c.province = p.id
		inner join district ds on c.district = ds.id
		LEFT JOIN prize_type pt on d.prize_type_id = pt.id`)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get draw data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetDraws: Unable to get draw data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "draw data is not valid")
	}
	for rows.Next() {
		draw := model.Draw{}
		var prizeTypeName *string
		var prizeTypeId, prizeTypeValue *int
		err = rows.Scan(&draw.Id, &draw.Code, &draw.Customer.Id, &draw.CreatedAt, &draw.Customer.Province.Id, &draw.Customer.Province.Name,
			&draw.Customer.District.Id, &draw.Customer.District.Name, &draw.Customer.CreatedAt, &prizeTypeName, &prizeTypeId, &prizeTypeValue,
			&draw.Customer.NetworkOperator, &draw.Customer.Locale)
		draw.Customer.Phone = "**********"
		draw.Customer.Names = "**********"
		if prizeTypeName != nil {
			draw.PrizeType.Name = *prizeTypeName
			draw.PrizeType.Id = *prizeTypeId
			draw.PrizeType.Value = *prizeTypeValue
		}
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get draw data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetEntries: Unable to get draw data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		draws = append(draws, draw)
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": draws})
}
func CreateNewDraw(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type FormData struct {
		Name         string `json:"name" binding:"required" validate:"required,regex=^[a-zA-Z0-9\\-_ #@]*$"`
		CategoryId   int    `json:"category_id" binding:"required" validate:"required,number"`
		Value        int    `json:"value" binding:"required" validate:"required,number"`
		Elligibility int    `json:"elligibility" binding:"required" validate:"required,number"`
	}
	responseStatus := 200
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil || formData.Name == "" {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data - " + formData.Name, "details": err})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provide data are not valid")
	}
	invalidKeys := utils.ValidateStruct(formData, []string{"#", "@"}, []string{})
	errorMessage := utils.ValidateStructText(invalidKeys)
	if errorMessage != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, *errorMessage)
	}
	_, err = config.DB.Exec(ctx,
		`insert into prize_type (name,prize_category_id,elligibility,value,status,operator_id) values ($1,$2,$3,$4,'OKAY',$5)`,
		formData.Name, formData.CategoryId, formData.Elligibility, formData.Value, userPayload.Id)
	if err != nil {
		if ok, key := utils.IsErrDuplicate(err); ok {
			return utils.JsonErrorResponse(c, fiber.StatusConflict, fmt.Sprintf("Unable to save data, %s already exists", key))
		} else if ok, key := utils.IsForeignKeyErr(err); ok {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, fmt.Sprintf("Unable to save data, %s is invalid", key))
		}
		responseStatus = fiber.StatusConflict
		c.SendStatus(responseStatus)
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save data, system error. please try again later", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("CreatePrizeType: Unable to save data, Name:%s, err:%v", formData.Name, err),
			ServiceName: config.ServiceName,
		})
	}
	return c.JSON(fiber.Map{"status": responseStatus, "message": "Prize type added successfully"})
}
func AddUser(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type FormData struct {
		Fname          string `json:"fname" binding:"required" validate:"required,regex=^[a-zA-Z0-9 ]*$"`
		Lname          string `json:"lname" binding:"required" validate:"required,regex=^[a-zA-Z0-9 ]*$"`
		Phone          string `json:"phone" binding:"required" validate:"required,regex=^07[2389]\\d{7}$"`
		Email          string `json:"email" binding:"required" validate:"required,email"`
		Department     int    `json:"department" binding:"required" validate:"required,number"`
		CanAddCode     bool   `json:"can_add_codes" binding:"required" validate:"boolean"`
		CanTriggerDraw bool   `json:"can_trigger_draw" binding:"required" validate:"boolean"`
		CanViewLogs    bool   `json:"can_view_logs" binding:"required" validate:"boolean"`
		CanAddUser     bool   `json:"can_add_user" binding:"required" validate:"boolean"`
	}
	responseStatus := 200
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil || formData.Fname == "" {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data - " + formData.Fname, "details": err})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provide data are not valid")
	}
	invalidKeys := utils.ValidateStruct(formData, []string{"#", "@"}, []string{})
	errorMessage := utils.ValidateStructText(invalidKeys)
	if errorMessage != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, *errorMessage)
	}
	n := utils.GenerateRandomNumber(89)
	avatarUrl := fmt.Sprintf("%s/api/v1/avatar/svg/av/%d", viper.GetString("BACKEND_URL"), n)
	//insert user data, and will have to set password for the first time with a verification using phone
	_, err = config.DB.Exec(ctx,
		`insert into users (fname,lname, email,phone, department_id, password, can_add_codes, can_trigger_draw, can_view_logs, can_add_user, status, operator, avatar_url) values ($1, $2, $3, $4, $5, '-', $6, $7, $8, $9, 'OKAY', $10, $11)`,
		formData.Fname, formData.Lname, formData.Email, formData.Phone, formData.Department, formData.CanAddCode, formData.CanTriggerDraw, formData.CanViewLogs, formData.CanAddUser, userPayload.Id, avatarUrl)

	if err != nil {
		if ok, key := utils.IsErrDuplicate(err); ok {
			return utils.JsonErrorResponse(c, fiber.StatusConflict, fmt.Sprintf("Unable to save data, %s already exists", key))
		} else if ok, key := utils.IsForeignKeyErr(err); ok {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, fmt.Sprintf("Unable to save data, %s is invalid", key))
		}
		responseStatus = fiber.StatusConflict
		c.SendStatus(responseStatus)
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save data, system error. please try again later", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("CreatePrizeType: Unable to save data, Name:%s, err:%v", formData.Fname, err),
			ServiceName: config.ServiceName,
		})
	}
	return c.JSON(fiber.Map{"status": responseStatus, "message": "User added successfully"})
}

func GetUsers(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	users := []model.UserProfile{}
	//fetch users
	rows, err := config.DB.Query(ctx,
		`select u.id,u.fname,u.lname,u.email,u.phone,u.department_id,d.title as department_title, u.email_verified,u.phone_verified,u.avatar_url,u.status,
			u.can_add_codes,u.can_trigger_draw,u.can_view_logs,u.can_add_user from users u inner join departments d on u.department_id = d.id`)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get users data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetUsers: Unable to get users data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "users data is not valid")
	}
	for rows.Next() {
		user := model.UserProfile{}
		//scan user data
		err = rows.Scan(&user.Id, &user.Fname, &user.Lname, &user.Email, &user.Phone, &user.Department.Id, &user.Department.Title, &user.EmailVerified, &user.PhoneVerified,
			&user.AvatarUrl, &user.Status, &user.CanAddCodes, &user.CanTriggerDraw, &user.CanViewLogs, &user.CanAddUser)

		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get users data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetUsers: Unable to get users data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		users = append(users, user)
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": users})
}

func GetCustomer(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	customerId := c.Params("customerId")
	customer := model.Customer{}
	fmt.Println("Secret key", config.EncryptionKey)
	err = config.DB.QueryRow(ctx,
		`select p.id as province_id,p.name as province_name,d.id as district_id,d.name as district_name,
		c.created_at,c.network_operator,c.locale,pgp_sym_decrypt(c.names::bytea,$1) as names,pgp_sym_decrypt(c.phone::bytea,$1) as phone,c.id from customer c
		inner join province p on c.province = p.id
		inner join district d on c.district = d.id where c.id=$2`, config.EncryptionKey, customerId).
		Scan(&customer.Province.Id, &customer.Province.Name, &customer.District.Id, &customer.District.Name, &customer.CreatedAt, &customer.NetworkOperator,
			&customer.Locale, &customer.Names, &customer.Phone, &customer.Id)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get customer data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetCustomer: Unable to get customer data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "customer id provided is not valid")
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": customer})
}

func GetEntryData(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	entryId := c.Params("entryId")
	entry := model.Entries{}
	fmt.Println("Secret key", config.EncryptionKey)
	var prizeTypeName *string
	var prizeTypeId, prizeTypeValue *int
	var PrizeDate *time.Time
	var prizeId *int
	err = config.DB.QueryRow(ctx,
		`select e.id,e.code_id,e.customer_id,e.created_at,p.id as province_id,p.name as province_name,d.id as district_id,d.name as district_name,
		c.created_at,pt.name as prize_type_name,pt.id as prize_type_id,pt.value as prize_type_value,cd.created_at,c.network_operator,c.locale,
		pgp_sym_decrypt(c.names::bytea,$1) as names,pgp_sym_decrypt(c.momo_names::bytea,$1) as momo_names,pgp_sym_decrypt(c.phone::bytea,$1) as phone,
		pgp_sym_decrypt(cd.code::bytea,$1) as raw_code,pr.created_at,pr.id as prize_id from entries e
		inner join customer c on e.customer_id = c.id
		inner join codes cd on e.code_id = cd.id
		inner join province p on c.province = p.id
		inner join district d on c.district = d.id
		LEFT join prize pr on pr.entry_id = e.id
		LEFT JOIN prize_type pt on cd.prize_type_id = pt.id where c.id=$2`, config.EncryptionKey, entryId).
		Scan(&entry.Id, &entry.Code.Id, &entry.Customer.Id, &entry.CreatedAt, &entry.Customer.Province.Id, &entry.Customer.Province.Name,
			&entry.Customer.District.Id, &entry.Customer.District.Name, &entry.Customer.CreatedAt, &prizeTypeName, &prizeTypeId, &prizeTypeValue,
			&entry.Code.CreatedAt, &entry.Customer.NetworkOperator, &entry.Customer.Locale, &entry.Customer.Names, &entry.Customer.MOMONames,
			&entry.Customer.Phone, &entry.Code.Code, &PrizeDate, &prizeId)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get customer entry data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetEntryData: Unable to get customer entry data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusNotFound, "entry id provided is not valid")
	}
	if prizeTypeName != nil {
		entry.Code.PrizeType = new(model.PrizeType)
		entry.Code.PrizeType.Name = *prizeTypeName
		entry.Code.PrizeType.Id = *prizeTypeId
		entry.Code.PrizeType.Value = *prizeTypeValue
		if prizeId != nil {
			entry.Prize = new(model.Prize)
			entry.Prize.CreatedAt = *PrizeDate
			entry.Prize.Id = *prizeId
			entry.Prize.Value = *prizeTypeValue
			entry.Prize.PrizeType = *entry.Code.PrizeType
		}
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": entry})
}
func ChangePassword(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type FormData struct {
		OldPassword string `json:"current_password" validate:"required"`
		NewPassword string `json:"new_password" validate:"required,min=8,max=50,strong_password"`
	}
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide all required data")
	}
	// Register the custom validation function for strong password
	err = Validate.RegisterValidation("strong_password", utils.IsStrongPassword)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Create project failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("SetNewPassword: Error registering custom validation: strong_password, Err: %s", err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "provided password is not strong")
	}
	var password, status string
	err = config.DB.QueryRow(ctx, "select password,status from users where id=$1", userPayload.Id).
		Scan(&password, &status)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Change password failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "Unable to verify user info, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		} else {
			return utils.JsonErrorResponse(c, fiber.StatusForbidden, "User data is not valid")
		}
	} else if err := bcrypt.CompareHashAndPassword([]byte(password), []byte(formData.OldPassword)); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Old password is incorrect")
	} else if status != "OKAY" {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Your account is not active")
	} else if err := bcrypt.CompareHashAndPassword([]byte(password), []byte(formData.NewPassword)); err == nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "New Password is the same as current one, no action made")
	}
	_, err = config.DB.Exec(ctx, "update users set password=crypt($1, gen_salt('bf')) where id=$2", formData.NewPassword, userPayload.Id)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Change password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("Unable to change password for %d! Err: %s", userPayload.Id, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	c.SendStatus(200)
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": fmt.Sprintf("Dear %s, you password changed successful", userPayload.Fname)})
}
func ForgotPassword(c *fiber.Ctx) error {
	type FormData struct {
		Email string `json:"email" validate:"required,email"`
	}
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide all required data")
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide all required data")
	}
	invalidKeys := utils.ValidateStruct(formData, []string{}, []string{"Password"})
	errorMessage := utils.ValidateStructText(invalidKeys)
	if errorMessage != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, *errorMessage)
	}
	uniqueResetTokenKey := base64.RawStdEncoding.EncodeToString([]byte(formData.Email + utils.RandString(20)))
	successResponse := c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "You will receive an email if we found an account match with this email",
		"reset_key": uniqueResetTokenKey, "email": formData.Email})
	var id, status, fname, lname, phone string
	err := config.DB.QueryRow(ctx, "select id,status,fname,lname,phone from users where email=$1 limit 1", formData.Email).
		Scan(&id, &status, &fname, &lname, &phone)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "ForgotPassword: Unable to verify user info, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		} else {
			//send a success message even if the email if not found to protect from email guessing
			return successResponse
		}
	} else if status != "OKAY" {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Your account is not active")
	}
	otp, err := utils.GenerateOTP(6)
	if utils.IsTestMode {
		otp = "123456"
	}
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "ForgotPassword: Unable to generate otp, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	//save key with otp and other validation info in redis
	otpData, err := json.Marshal(map[string]any{
		"otp":        otp,
		"email":      formData.Email,
		"phone":      phone,
		"userId":     id,
		"fname":      fname,
		"lname":      lname,
		"created_at": time.Now(),
	})
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ForgotPassword: unable to marshal payload data for email %s, error:%s ", formData.Email, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	err = config.Redis.Set(c.Context(), uniqueResetTokenKey, otpData, time.Minute*20).Err()
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ForgotPassword: unable to save redis data for email %s, error:%s ", formData.Email, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	//send email containing otp
	go utils.SendSMS(phone, fmt.Sprintf("Dear %s, %s is the OTP for reseting your password. don't share it with anyone.", fname, otp), viper.GetString("SENDER_ID"), config.ServiceName, "reset_password_otp", nil)
	return successResponse
}
func ValidateOTP(c *fiber.Ctx) error {
	type FormData struct {
		Otp      string `json:"otp" validate:"required"`
		ResetKey string `json:"reset_key" validate:"required"`
	}
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide all required data")
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide all required data with a valid format")
	}
	invalidKeys := utils.ValidateStruct(formData, []string{}, []string{})
	errorMessage := utils.ValidateStructText(invalidKeys)
	if errorMessage != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, *errorMessage)
	}
	otpTxtData, err := config.Redis.Get(c.Context(), formData.ResetKey).Result()
	if err == redis.Nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid or expired OTP provided")
	} else if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ValidateOTP: unable to fetch otp data from redis, reset_key: %s, error:%s ", formData.ResetKey, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	otpData := make(map[string]any)
	err = json.Unmarshal([]byte(otpTxtData), &otpData)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ValidateOTP: unable to unmarshal payload data, Data: %s, error:%s ", otpTxtData, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	if otpData["otp"].(string) != formData.Otp {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid OTP provided")
	}
	if strings.Contains(c.Route().Path, "/verify_otp") {
		//mark phone as verified
		_, err := config.DB.Exec(ctx, "update users set phone_verified=true where id=$1", otpData["userId"])
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Verify OTP failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     fmt.Sprintf("ValidateOTP: unable to unmarshal payload data, Data: %s, error:%s ", otpTxtData, err.Error()),
				ServiceName: config.ServiceName,
			})
		}
		err = config.Redis.Del(c.Context(), formData.ResetKey).Err()
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Validate OTP failed, please restart the action again", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     fmt.Sprintf("ValidateOTP: unable to delete redis data for email %s, error:%s ", otpData["email"], err.Error()),
				ServiceName: config.ServiceName,
			})
		}
		return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "Phone is verified",
			"email": otpData["email"], "phone": otpData["phone"], "user_id": otpData["userId"]})
	}
	uniqueResetTokenKey := base64.RawStdEncoding.EncodeToString([]byte(otpData["email"].(string) + utils.RandString(20)))
	//save key with email and other validation info in redis
	resetPasswordData, err := json.Marshal(map[string]any{
		"email":      otpData["email"],
		"phone":      otpData["phone"],
		"userId":     otpData["userId"],
		"fname":      otpData["fname"],
		"lname":      otpData["lname"],
		"created_at": time.Now(),
	})
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ValidateOTP: unable to marshal payload data for email %s, error:%s ", otpData["email"], err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	err = config.Redis.Set(c.Context(), uniqueResetTokenKey, resetPasswordData, time.Minute*20).Err()
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Validate OTP failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ValidateOTP: unable to save redis data for email %s, error:%s ", otpData["email"], err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	err = config.Redis.Del(c.Context(), formData.ResetKey).Err()
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Validate OTP failed, please restart the action again", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("ValidateOTP: unable to delete redis data for email %s, error:%s ", otpData["email"], err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "OTP is valid, set your new password", "reset_key": uniqueResetTokenKey,
		"email": otpData["email"]})
}

func SetNewPassword(c *fiber.Ctx) error {
	type FormData struct {
		Password string `json:"password" validate:"required,min=8,max=50,strong_password"`
		ResetKey string `json:"reset_key" validate:"required"`
	}
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide all required data")
	}
	// Register the custom validation function for strong password
	err := Validate.RegisterValidation("strong_password", utils.IsStrongPassword)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "The provided password is weak!", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("SetNewPassword: Error registering custom validation: strong_password, Err: %s", err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	invalidKeys := utils.ValidateStruct(formData, []string{}, []string{"Password"})
	errorMessage := utils.ValidateStructText(invalidKeys)
	if errorMessage != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, *errorMessage)
	}
	otpTxtData, err := config.Redis.Get(c.Context(), formData.ResetKey).Result()
	if err == redis.Nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Unable to reset password, invalid verify key")
	} else if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("SetNewPassword: unable to fetch reset password data from redis, reset_key: %s, error:%s ", formData.ResetKey, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	resetData := make(map[string]any)
	err = json.Unmarshal([]byte(otpTxtData), &resetData)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("SetNewPassword: unable to unmarshal payload data, Data: %s, error:%s ", otpTxtData, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	//update password
	_, err = config.DB.Exec(ctx, "update users set password=crypt($1, gen_salt('bf')) where id=$2", formData.Password, resetData["userId"])
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Reset password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("SetNewPassword: unable to update password, Email: %s, userId: %s, error:%s ", resetData["email"], resetData["userId"], err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	//delete reset key
	err = config.Redis.Del(c.Context(), formData.ResetKey).Err()
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Validate OTP failed, please restart the action again", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("SetNewPassword: unable to delete redis data for email %s, error:%s ", resetData["email"], err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "Password reset completed", "email": resetData["email"]})
}
func SendVerificationEmail(c *fiber.Ctx) error {
	type FormData struct {
		Email string `json:"email" validate:"required,email"`
	}
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide all required data")
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide all required data")
	}
	uniqueResetTokenKey := base64.RawStdEncoding.EncodeToString([]byte(formData.Email + utils.RandString(20)))
	var id, status, fname, phone, lname string
	var email_verified bool
	err := config.DB.QueryRow(ctx, "select id,status,email_verified,phone,fname,lname from users where email=$1 limit 1", formData.Email).
		Scan(&id, &status, &email_verified, &phone, &fname, &lname)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Sending verification otp failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "SendVerificationEmail: Unable to verify user info, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		} else {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Not account found for the provided email")
		}
	} else if status != "OKAY" {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "Your account is not active")
	} else if email_verified {
		c.SendStatus(fiber.StatusAccepted)
		return c.JSON(fiber.Map{"status": fiber.StatusAccepted, "message": "This email is already verified, nothing to do",
			"reset_key": uniqueResetTokenKey, "email": formData.Email})
	}
	otp, err := utils.GenerateOTP(6)
	if utils.IsTestMode {
		otp = "123456"
	}
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Sending verification otp failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "SendVerificationEmail: Unable to generate otp, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	//save key with otp and other validation info in redis
	otpData, err := json.Marshal(map[string]any{
		"otp":        otp,
		"email":      formData.Email,
		"userId":     id,
		"fname":      fname,
		"lname":      lname,
		"created_at": time.Now(),
	})
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Sending verification otp failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("SendVerificationEmail: unable to marshal payload data for email %s, error:%s ", formData.Email, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	err = config.Redis.Set(c.Context(), uniqueResetTokenKey, otpData, time.Minute*20).Err()
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Sending verification otp failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("SendVerificationEmail: unable to save redis data for email %s, error:%s ", formData.Email, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	type EmailData struct {
		Otp   string
		Email string
		Phone string
		Names string
	}
	data := EmailData{
		Otp:   otp,
		Email: formData.Email,
		Phone: phone,
		Names: fname + " " + lname,
	}
	body, err := utils.GenerateHtmlTemplate("verify_account_otp.html", data)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Sending verification otp failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("SendVerificationEmail: unable to generate email template for email %s, error:%s ", formData.Email, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	//send email containing otp
	utils.SendEmail(formData.Email, "Verify your account", body, config.ServiceName)
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "You will receive an email contains the OTP for verification, it will be expired in 20 minutes",
		"reset_key": uniqueResetTokenKey, "email": formData.Email})
}
