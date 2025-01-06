package controller

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"logger-service/helper"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"web-service/config"
	"web-service/model"

	"shared-package/utils"

	"math/rand"

	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/spf13/viper"
	"github.com/xuri/excelize/v2"
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
	u.can_add_codes,u.can_trigger_draw,u.can_add_user,u.can_view_logs,phone,force_change_password from users u inner join departments d on u.department_id = d.id where email = $1 and password = crypt($2, password)`, userData.Email, userData.Password).
		Scan(&UserProfile.Id, &UserProfile.Fname, &UserProfile.Lname, &UserProfile.Department.Id, &UserProfile.Department.Title, &UserProfile.EmailVerified, &UserProfile.PhoneVerified, &UserProfile.AvatarUrl, &UserProfile.Status,
			&UserProfile.CanAddCodes, &UserProfile.CanTriggerDraw, &UserProfile.CanAddUser, &UserProfile.CanViewLogs, &UserProfile.Phone, &UserProfile.ForceChangePassword)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			utils.LogMessage("critical", fmt.Sprintf("LoginWithEmail: Unable to get user data, Email:%s, err:%v", userData.Email, err), "web-service")
		}
		responseStatus = 403
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Invalid credentials"})
	} else if UserProfile.Status == "inactive" {
		utils.RecordActivityLog(config.DB,
			utils.ActivityLog{
				UserID:       UserProfile.Id,
				ActivityType: "LoginWithEmail",
				Description:  "Login failed, user account not active",
				Status:       "failure",
				IPAddress:    c.IP(),
				UserAgent:    c.Get("User-Agent"),
			},
			config.ServiceName,
			&map[string]interface{}{
				"email": userData.Email,
			},
		)
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
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       UserProfile.Id,
			ActivityType: "LoginWithEmail",
			Description:  "successful login",
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"email": userData.Email,
		},
	)
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
	u.can_add_codes,u.can_trigger_draw,u.can_add_user,u.can_view_logs,u.phone,u.force_change_password from users u inner join departments d on u.department_id = d.id where u.id = $1`, userPayload.Id).
		Scan(&UserProfile.Id, &UserProfile.Fname, &UserProfile.Lname, &UserProfile.Department.Id, &UserProfile.Department.Title, &UserProfile.EmailVerified, &UserProfile.PhoneVerified, &UserProfile.AvatarUrl, &UserProfile.Status,
			&UserProfile.CanAddCodes, &UserProfile.CanTriggerDraw, &UserProfile.CanAddUser, &UserProfile.CanViewLogs, &UserProfile.Phone, &UserProfile.ForceChangePassword)
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
		`select id,name,status,created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' from prize_category`)
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
			`select p.id,p.name,p.status,p.value,p.elligibility,pc.name as category_name,pc.id as category_id,pc.status as category_status,pc.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',
			p.period,p.distribution_type,p.expiry_date,STRING_AGG(pm.lang, ', ') as langs,STRING_AGG(pm.message, ', ') as messages,trigger_by_system from prize_type p join prize_category pc on p.prize_category_id = pc.id join prize_message pm on pm.prize_type_id=p.id group by p.id,pc.id`)
	} else {
		rows, err = config.DB.Query(ctx,
			`select p.id,p.name,p.status,p.value,p.elligibility,pc.name as category_name,pc.id as category_id,pc.status as category_status,pc.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',
			p.period,p.distribution_type,p.expiry_date,STRING_AGG(pm.lang, ', ') as langs,STRING_AGG(pm.message, ', ') as messages,trigger_by_system from prize_type p join prize_category pc on p.prize_category_id = pc.id join prize_message pm on pm.prize_type_id=p.id where p.prize_category_id=$1 group by p.id,pc.id`, prizeCategory)
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
			&prize.PrizeCategory.Id, &prize.PrizeCategory.Status, &prize.PrizeCategory.CreatedAt, &prize.CreatedAt, &prize.Period,
			&prize.Distribution, &prize.ExpiryDate, &langs, &messages, &prize.TriggerBySystem)
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
func GetPrizeTypeSpace(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	prizeTypeId, err := c.ParamsInt("type_id")
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Provide type id is not valid")
	}
	prizeType := model.PrizeType{}
	err = config.DB.QueryRow(ctx,
		`select p.id,p.name,p.status,p.value,p.elligibility,p.period from prize_type p where p.id=$1`, prizeTypeId).
		Scan(&prizeType.Id, &prizeType.Name, &prizeType.Status, &prizeType.Value, &prizeType.Elligibility, &prizeType.Period)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prize type data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeTypeSpace: Unable to get prize type data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "prize type data is not valid")
	}
	//get occupied prize space based on prize type period
	prizeFilter := ""
	if prizeType.Period == "MONTHLY" {
		prizeFilter = "created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= now() - interval '1 month'"
	} else if prizeType.Period == "WEEKLY" {
		prizeFilter = "created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= now() - interval '6 day'"
	} else if prizeType.Period == "DAILY" {
		prizeFilter = "created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= now() - interval '1 day'"
	}
	if prizeFilter != "" {
		prizeFilter = " and " + prizeFilter
	}
	occupiedSpace := 0
	err = config.DB.QueryRow(ctx,
		`select count(*) from prize where prize_type_id=$1`+prizeFilter, prizeTypeId).
		Scan(&occupiedSpace)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prize type data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeTypeSpace: Unable to get prize type data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	remaining := prizeType.Elligibility - occupiedSpace
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "name": prizeType.Name, "elligibility": prizeType.Elligibility, "occupied": occupiedSpace, "remaining": remaining})
}
func GetEntries(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	provinceId := c.Query("province_id")
	networkOperator := c.Query("network_operator")
	//add pagination
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	offSet := (page - 1) * limit

	err = utils.ValidateDateRanges(startDateStr, &endDateStr)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, err.Error())
	}
	if len(provinceId) != 0 {
		//check if user id is valid integer
		_, err := strconv.Atoi(provinceId)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid province id provided")
		}
	}

	args1 := []interface{}{}
	// limitStr := fmt.Sprintf(" limit $%d offset $%d", a+1, a+2)
	logsFilter, ii := utils.BuildQueryFilter(
		map[string]interface{}{
			"e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= ": startDateStr,
			"e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= ": endDateStr,
			"c.province":         provinceId,
			"c.network_operator": networkOperator,
		},
		&args1,
	)
	limitStr := fmt.Sprintf(" limit $%d offset $%d", ii, ii+1)
	globalArgs := args1
	args1 = append(args1, limit, offSet)
	entries := []model.Entries{}
	rows, err := config.DB.Query(ctx,
		`select e.id,e.code_id,e.customer_id,e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',p.id as province_id,p.name as province_name,d.id as district_id,d.name as district_name,
		c.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',pt.name as prize_type_name,pt.id as prize_type_id,pt.value as prize_type_value,cd.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',c.network_operator,c.locale from entries e
		inner join customer c on e.customer_id = c.id
		inner join codes cd on e.code_id = cd.id
		inner join province p on c.province = p.id
		inner join district d on c.district = d.id
		LEFT JOIN prize_type pt on cd.prize_type_id = pt.id `+logsFilter+` order by e.id desc`+limitStr, args1...)
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
	totalEntries := 0
	err = config.DB.QueryRow(ctx,
		`select count(e.id) from entries e inner join customer c on e.customer_id = c.id `+logsFilter, globalArgs...).Scan(&totalEntries)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get entries data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetEntries: Unable to get total entries data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": entries,
		"pagination": fiber.Map{"page": page, "limit": limit, "total": totalEntries}})
}

func GetPrizes(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	typeId := c.Query("type_id")
	code := c.Query("code")
	//add pagination
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	offSet := (page - 1) * limit

	err = utils.ValidateDateRanges(startDateStr, &endDateStr)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, err.Error())
	}
	if len(typeId) != 0 {
		//check if user id is valid integer
		_, err := strconv.Atoi(typeId)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid type id provided")
		}
	}

	args1 := []interface{}{}
	// limitStr := fmt.Sprintf(" limit $%d offset $%d", a+1, a+2)
	logsFilter, ii := utils.BuildQueryFilter(
		map[string]interface{}{
			"p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= ": startDateStr,
			"p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= ": endDateStr,
			"p.prize_type_id": typeId,
			"p.code":          code,
		},
		&args1,
	)
	limitStr := fmt.Sprintf(" limit $%d offset $%d", ii, ii+1)
	globalArgs := args1
	args1 = append(args1, limit, offSet)
	prizes := []model.Prize{}
	rows, err := config.DB.Query(ctx,
		`select p.id,p.rewarded,p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',p.prize_value,p.prize_type_id,pc.name as category_name,pc.status as category_status,pc.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' as category_created_at,
		e.customer_id,pt.name,pc.id,p.code from prize p
		inner join entries e on p.entry_id = e.id
		inner join prize_type pt on pt.id = p.prize_type_id
		inner join prize_category pc on pt.prize_category_id = pc.id `+logsFilter+` order by p.id desc`+limitStr, args1...)
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
		err = rows.Scan(&prize.Id, &prize.Rewarded, &prize.CreatedAt, &prize.Value, &prize.PrizeType.Id, &prize.PrizeCategory.Name, &prize.PrizeCategory.Status,
			&prize.PrizeCategory.CreatedAt, &prize.Customer.Id, &prize.PrizeType.Name, &prize.PrizeCategory.Id, &prize.Code)
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
	totalPrizes := 0
	err = config.DB.QueryRow(ctx,
		`select count(id) from prize p `+logsFilter, globalArgs...).Scan(&totalPrizes)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prizes data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizes: Unable to get total prizes data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": prizes,
		"pagination": fiber.Map{"page": page, "limit": limit, "total": totalPrizes}})
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
			utils.RecordActivityLog(config.DB,
				utils.ActivityLog{
					UserID:       userPayload.Id,
					ActivityType: "CreatePrizeCategory",
					Description:  "adding a new prize category: " + formData.Name + " failed, duplicate",
					Status:       "failure",
					IPAddress:    c.IP(),
					UserAgent:    c.Get("User-Agent"),
				},
				config.ServiceName,
				nil,
			)
			return utils.JsonErrorResponse(c, fiber.StatusConflict, fmt.Sprintf("Unable to save data, %s already exists", key))
		}
		utils.RecordActivityLog(config.DB,
			utils.ActivityLog{
				UserID:       userPayload.Id,
				ActivityType: "CreatePrizeCategory",
				Description:  "adding a new prize category: " + formData.Name + " failed, duplicate",
				Status:       "failure",
				IPAddress:    c.IP(),
				UserAgent:    c.Get("User-Agent"),
			},
			config.ServiceName,
			nil,
		)
		responseStatus = fiber.StatusConflict
		c.SendStatus(responseStatus)
		return utils.JsonErrorResponse(c, fiber.StatusConflict, "Unable to save data, system error. please try again later", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("CreatePrizeCategory: Unable to save data, Name:%s, err:%v", formData.Name, err),
			ServiceName: config.ServiceName,
		})
	}
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "CreatePrizeCategory",
			Description:  "added a new prize category: " + formData.Name,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		nil,
	)
	return c.JSON(fiber.Map{"status": responseStatus, "message": "Prize category added successfully"})
}
func CreatePrizeType(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}

	type FormData struct {
		Id              int                  `json:"id" binding:"required" validate:"number"`
		Name            string               `json:"name" binding:"required" validate:"required,regex=^[a-zA-Z0-9\\-_ #@]*$"`
		CategoryId      int                  `json:"prize_category" binding:"required" validate:"required,number"`
		Value           int                  `json:"value" binding:"required" validate:"required,number"`
		Elligibility    int                  `json:"elligibility" binding:"required" validate:"required,number"`
		ExpiryDate      time.Time            `json:"expiry_date" binding:"required" validate:"required"`
		Period          string               `json:"period" binding:"required" validate:"required,oneof=MONTHLY WEEKLY DAILY GRAND"`
		Distribution    string               `json:"distribution" binding:"required" validate:"required,oneof=momo cheque other"`
		TriggerBySystem bool                 `json:"trigger_by_system" binding:"required" validate:"boolean"`
		Messages        []model.PrizeMessage `json:"messages" binding:"required" validate:"required"`
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
	formData.Period = strings.ToUpper(formData.Period)
	//check if expiry_date is already expired
	// expiryDate, err := time.Parse("02/01/2006", formData.ExpiryDate)
	// if err != nil {
	// 	return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provided expiry_date is not valid, please use this format dd/mm/yyyy")
	// }
	if formData.ExpiryDate.Before(time.Now()) {
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
	var oldName, oldValue, oldElligibility, oldPeriod string
	var oldExpiry time.Time
	actionType := "Create"
	//check if formdata id is valid
	if formData.Id != 0 {
		actionType = "Update"
		if err = tx.QueryRow(ctx,
			`select id,name,value,elligibility,expiry_date,period from prize_type where id = $1`, formData.Id).
			Scan(&prizeTypeId, &oldName, &oldValue, &oldElligibility, &oldExpiry, &oldPeriod); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return utils.JsonErrorResponse(c, fiber.StatusConflict, "Provided id is not valid")
			} else {
				return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save data, system error. please try again later", utils.Logger{
					LogLevel:    utils.CRITICAL,
					Message:     fmt.Sprintf("CreatePrizeType: Unable to check id, Name:%s, err:%v", formData.Name, err),
					ServiceName: config.ServiceName,
				})
			}
		}
		//update prize type
		_, err = tx.Exec(ctx,
			`update prize_type set name=$1,prize_category_id=$2,elligibility=$3,value=$4,status='OKAY',operator_id=$5,expiry_date=$6,distribution_type=$7,period=$8,trigger_by_system=$9 where id=$10`,
			formData.Name, formData.CategoryId, formData.Elligibility, formData.Value, userPayload.Id, formData.ExpiryDate, formData.Distribution, formData.Period,
			formData.TriggerBySystem, formData.Id)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save data, system error. please try again later", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     fmt.Sprintf("CreatePrizeType: Unable to update data, Name:%s, err:%v", formData.Name, err),
				ServiceName: config.ServiceName,
			})
		}
	} else {
		err = tx.QueryRow(ctx,
			`insert into prize_type (name,prize_category_id,elligibility,value,status,operator_id,expiry_date,distribution_type,period,trigger_by_system)
		values ($1,$2,$3,$4,'OKAY',$5, $6, $7, $8, $9)  returning id`,
			formData.Name, formData.CategoryId, formData.Elligibility, formData.Value, userPayload.Id, formData.ExpiryDate, formData.Distribution, formData.Period,
			formData.TriggerBySystem).
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
	}
	//save message
	for _, message := range formData.Messages {
		//change to insert or update
		_, err = tx.Exec(ctx,
			`insert into prize_message (lang, prize_type_id, message, operator_id)
			values ($1, $2, $3, $4)
			on conflict (lang, prize_type_id) do update
			set message = $3,
				operator_id = $4`,
			message.Lang, prizeTypeId, message.Message, userPayload.Id)
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
	logMessage := "added a new prize type: " + formData.Name
	returnMessage := "Prize type added successfully"
	if actionType == "Update" {
		returnMessage = oldName + " updated successfully"
		if oldName != formData.Name {
			logMessage += " (old name: " + oldName + ")"
		}
		if oldValue != fmt.Sprint(formData.Value) {
			logMessage += " (old value: " + oldValue + ")"
		}
		if oldElligibility != fmt.Sprint(formData.Elligibility) {
			logMessage += " (old elligibility: " + oldElligibility + ")"
		}
		if oldExpiry != formData.ExpiryDate {
			logMessage += " (old expiry date: " + oldExpiry.Format("02/01/2006") + ")"
		}
		if oldPeriod != formData.Period {
			logMessage += " (old period: " + oldPeriod + ")"
		}
	}
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: actionType + "PrizeType",
			Description:  logMessage,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"id": prizeTypeId,
		},
	)
	return c.JSON(fiber.Map{"status": responseStatus, "message": returnMessage})
}
func GetDraws(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	code := c.Query("code")
	//add pagination
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	offSet := (page - 1) * limit

	err = utils.ValidateDateRanges(startDateStr, &endDateStr)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, err.Error())
	}

	args1 := []interface{}{}
	// limitStr := fmt.Sprintf(" limit $%d offset $%d", a+1, a+2)
	logsFilter, ii := utils.BuildQueryFilter(
		map[string]interface{}{
			"d.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= ": startDateStr,
			"d.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= ": endDateStr,
			"d.code": code,
		},
		&args1,
	)
	limitStr := fmt.Sprintf(" limit $%d offset $%d", ii, ii+1)
	globalArgs := args1
	args1 = append(args1, limit, offSet)
	draws := []model.Draw{}
	rows, err := config.DB.Query(ctx,
		`select d.id,d.code,d.customer_id,d.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',d.status,p.id as province_id,p.name as province_name,ds.id as district_id,ds.name as district_name,
		c.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',pt.name as prize_type_name,pt.id as prize_type_id,pt.value as prize_type_value,c.network_operator,c.locale from draw d
		inner join customer c on d.customer_id = c.id
		inner join province p on c.province = p.id
		inner join district ds on c.district = ds.id
		LEFT JOIN prize_type pt on d.prize_type_id = pt.id`+logsFilter+` order by d.id desc`+limitStr, args1...)
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
		err = rows.Scan(&draw.Id, &draw.Code, &draw.Customer.Id, &draw.CreatedAt, &draw.Status, &draw.Customer.Province.Id, &draw.Customer.Province.Name,
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
				Message:     "GetDraws: Unable to get draw data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		draws = append(draws, draw)
	}
	totalDraws := 0
	err = config.DB.QueryRow(ctx,
		`select count(id) from draw d `+logsFilter, globalArgs...).Scan(&totalDraws)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get draws data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetDraws: Unable to get total draws data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": draws,
		"pagination": fiber.Map{"page": page, "limit": limit, "total": totalDraws}})
}
func AddUser(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	if !userPayload.CanAddUser {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "You don't have permission to add a user")
	}
	type FormData struct {
		Fname          string `json:"fname" binding:"required" validate:"required,regex=^[a-zA-Z0-9 ]*$"`
		Lname          string `json:"lname" binding:"required" validate:"required,regex=^[a-zA-Z0-9 ]*$"`
		Phone          string `json:"phone" binding:"required" validate:"required,regex=^2507[2389]\\d{7}$"`
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
	//insert user data, and will have to change password for the first time with a verification using phone
	rawPassword := utils.RandString(8)
	password, err := utils.HashPassword(rawPassword)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save data, system error. please try again later", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("AddUser: Unable to hash password, Name:%s, err:%v", formData.Fname, err),
			ServiceName: config.ServiceName,
		})
	}
	_, err = config.DB.Exec(ctx,
		`insert into users (fname,lname, email,phone, department_id, password, can_add_codes, can_trigger_draw, can_view_logs, can_add_user, status,force_change_password, operator, avatar_url) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'OKAY', true, $11, $12)`,
		formData.Fname, formData.Lname, formData.Email, formData.Phone, formData.Department, password, formData.CanAddCode, formData.CanTriggerDraw, formData.CanViewLogs, formData.CanAddUser, userPayload.Id, avatarUrl)

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
	//send password to user phone (sms)
	go utils.SendSMS(config.DB, formData.Phone, fmt.Sprintf("Your password is %s, please change it after login\n\n%s", rawPassword, viper.GetString("ap_name")), viper.GetString("SENDER_ID"), config.ServiceName, "account_password", nil, config.Redis)
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "addUser",
			Description:  "added a new user, names: " + formData.Fname + " " + formData.Lname + ", phone: " + formData.Phone + ", Email: " + formData.Email,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		nil,
	)
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
			u.can_add_codes,u.can_trigger_draw,u.can_view_logs,u.can_add_user,force_change_password from users u inner join departments d on u.department_id = d.id`)
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
			&user.AvatarUrl, &user.Status, &user.CanAddCodes, &user.CanTriggerDraw, &user.CanViewLogs, &user.CanAddUser, &user.ForceChangePassword)

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
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	customerId := c.Params("customerId")
	customer := model.Customer{}
	err = config.DB.QueryRow(ctx,
		`select p.id as province_id,p.name as province_name,d.id as district_id,d.name as district_name,
		c.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',c.network_operator,c.locale,pgp_sym_decrypt(c.names::bytea,$1) as names,pgp_sym_decrypt(c.phone::bytea,$1) as phone,c.id from customer c
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
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "viewCustomer",
			Description:  "View customer data, name: " + customer.Names,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"customer_id": customerId,
		},
	)
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": customer})
}

func GetEntryData(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	entryId := c.Params("entryId")
	entry := model.Entries{}
	var prizeTypeName *string
	var prizeTypeId, prizeTypeValue *int
	var PrizeDate *time.Time
	var prizeId *int
	err = config.DB.QueryRow(ctx,
		`select e.id,e.code_id,e.customer_id,e.created_at,p.id as province_id,p.name as province_name,d.id as district_id,d.name as district_name,
		c.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',pt.name as prize_type_name,pt.id as prize_type_id,pt.value as prize_type_value,cd.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',c.network_operator,c.locale,
		pgp_sym_decrypt(c.names::bytea,$1) as names,pgp_sym_decrypt(c.momo_names::bytea,$1) as momo_names,pgp_sym_decrypt(c.phone::bytea,$1) as phone,
		pgp_sym_decrypt(cd.code::bytea,$1) as raw_code,pr.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',pr.id as prize_id from entries e
		inner join customer c on e.customer_id = c.id
		inner join codes cd on e.code_id = cd.id
		inner join province p on c.province = p.id
		inner join district d on c.district = d.id
		LEFT join prize pr on pr.entry_id = e.id
		LEFT JOIN prize_type pt on pr.prize_type_id = pt.id where e.id=$2`, config.EncryptionKey, entryId).
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
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "viewEntryData",
			Description:  "View entry data, customer name: " + entry.Customer.Names,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"entry_id":    entry.Id,
			"customer_id": entry.Customer.Id,
		},
	)
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
	_, err = config.DB.Exec(ctx, "update users set password=crypt($1, gen_salt('bf')),force_change_password=false where id=$2", formData.NewPassword, userPayload.Id)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Change password failed", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("Unable to change password for %d! Err: %s", userPayload.Id, err.Error()),
			ServiceName: config.ServiceName,
		})
	}
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "changePassword",
			Description:  "Self: changed password",
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		nil,
	)
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
	var id int
	var status, fname, lname, phone string
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
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       id,
			ActivityType: "forgotPassword",
			Description:  "Self: Initiated a forgot password",
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		nil,
	)
	//send email containing otp
	go utils.SendSMS(config.DB, phone, fmt.Sprintf("Dear %s, %s is the OTP for reseting your password. don't share it with anyone.", fname, otp), viper.GetString("SENDER_ID"), config.ServiceName, "reset_password_otp", nil, config.Redis)
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
		utils.RecordActivityLog(config.DB,
			utils.ActivityLog{
				UserID:       int(otpData["userId"].(float64)),
				ActivityType: "validateOtp",
				Description:  "Self: OTP validated",
				Status:       "success",
				IPAddress:    c.IP(),
				UserAgent:    c.Get("User-Agent"),
			},
			config.ServiceName,
			nil,
		)
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
	_, err = config.DB.Exec(ctx, "update users set password=crypt($1, gen_salt('bf')),force_change_password=false where id=$2", formData.Password, resetData["userId"])
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
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       int(resetData["userId"].(float64)),
			ActivityType: "setNewPassword",
			Description:  "Self: Set new password",
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		nil,
	)
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
	var id int
	var status, fname, phone, lname string
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
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       id,
			ActivityType: "setNewPassword",
			Description:  "Self: Set new password",
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		nil,
	)
	//send email containing otp
	utils.SendEmail(formData.Email, "Verify your account", body, config.ServiceName)
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "You will receive an email contains the OTP for verification, it will be expired in 20 minutes",
		"reset_key": uniqueResetTokenKey, "email": formData.Email})
}
func StartPrizeDraw(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	if !userPayload.CanTriggerDraw {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "You don't have permission to start a draw")
	}
	type FormData struct {
		PrizeType uint `json:"prize_type" validate:"required,number"`
	}
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide all required data:"+err.Error())
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Provided prize type is invalid")
	}
	var name, status, period, distributionType string
	var id int
	var value float64
	var expiryDate *time.Time
	var triggerBySystem bool
	err = config.DB.QueryRow(ctx, "select id,name,status,expiry_date,trigger_by_system,period,value,distribution_type from prize_type where id=$1", formData.PrizeType).
		Scan(&id, &name, &status, &expiryDate, &triggerBySystem, &period, &value, &distributionType)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "StartPrizeDraw: Unable to fetch prize type data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		} else {
			return utils.JsonErrorResponse(c, fiber.StatusForbidden, "Provided prize type is invalid")
		}
	}
	if status != "OKAY" {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Selected prize type is not active")
	} else if expiryDate != nil && time.Now().After(*expiryDate) {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Selected prize type is expired")
	} else if triggerBySystem {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Selected prize can only be triggered by the system")
	}
	entryFilter := ""
	excludeCustomers := []string{}
	if period == "MONTHLY" {
		entryFilter = "e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= now() - interval '1 month'"
	} else if period == "WEEKLY" {
		entryFilter = "e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= now() - interval '6 day' AND e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' < date_trunc('day', now())"
	} else if period == "DAILY" {
		entryFilter = "e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= now() - interval '1 day'"
	} else if period == "GRAND" {
		//exclude all monthly winners
		rows, err := config.DB.Query(ctx,
			`select e.customer_id from prize p INNER JOIN entries e ON e.id=p.entry_id INNER JOIN prize_type pt on pt.id=p.prize_type_id
			where pt.period='MONTHLY' group by e.customer_id`)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
					LogLevel:    utils.CRITICAL,
					Message:     "StartPrizeDraw: Unable to fetch prize type data, error: " + err.Error(),
					ServiceName: config.ServiceName,
				})
			}
		}
		for rows.Next() {
			var prizeCustomerId string
			err = rows.Scan(&prizeCustomerId)
			if err != nil {
				return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
					LogLevel:    utils.CRITICAL,
					Message:     "StartPrizeDraw: Unable to scan prize type data, error: " + err.Error(),
					ServiceName: config.ServiceName,
				})
			}
			excludeCustomers = append(excludeCustomers, prizeCustomerId)
		}
	}
	//fetch latest prizes (customerId) for the selected prize type
	rows, err := config.DB.Query(ctx,
		`select e.customer_id from prize p INNER JOIN entries e on e.id = p.entry_id where p.prize_type_id=$1 and p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= now() - interval '1 day'`, formData.PrizeType)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "StartPrizeDraw: Unable to fetch latest prize data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	for rows.Next() {
		var customerId string
		err = rows.Scan(&customerId)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "StartPrizeDraw: Unable to scan latest prize data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		excludeCustomers = append(excludeCustomers, customerId)
	}

	if len(excludeCustomers) != 0 {
		if len(entryFilter) != 0 {
			entryFilter += " and "

		}
		entryFilter += fmt.Sprintf("e.customer_id not in ('%s')", strings.Join(excludeCustomers, "','"))
	}
	finalFilter := ""
	if len(entryFilter) != 0 {
		finalFilter = " where " + entryFilter
	}
	//get elligible entries
	entries := []model.Entries{}
	entryRows, err := config.DB.Query(ctx,
		`select e.id,e.code_id,e.customer_id,e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' from entries e `+finalFilter)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusExpectationFailed, "No elligible entries found for the selected prize type")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "StartPrizeDraw: Unable to fetch entries data, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	for entryRows.Next() {
		entry := model.Entries{}
		err = entryRows.Scan(&entry.Id, &entry.Code.Id, &entry.Customer.Id, &entry.CreatedAt)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "StartPrizeDraw: Unable to scan entries, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return utils.JsonErrorResponse(c, fiber.StatusExpectationFailed, "No elligible entries found for the selected prize type")
	}
	//select a random entry
	randomGen := rand.New(rand.NewSource(time.Now().UnixNano()))
	selectedEntry := entries[randomGen.Intn(len(entries))]
	//get customer data
	var customerPhone, customerName, customerLocale, mno string
	err = config.DB.QueryRow(ctx, "select pgp_sym_decrypt(phone::bytea,$1), pgp_sym_decrypt(names::bytea,$1),locale,network_operator from customer where id=$2",
		config.EncryptionKey, selectedEntry.Customer.Id).
		Scan(&customerPhone, &customerName, &customerLocale, &mno)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "StartPrizeDraw: Unable to fetch customer info, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	//get prize message based on customer locale
	prizeMessage := ""
	err = config.DB.QueryRow(ctx, "select message from prize_message where prize_type_id=$1 and lang=$2", id, customerLocale).
		Scan(&prizeMessage)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusExpectationFailed, "Unable to start a new draw, no prize sms available for the selected prize type")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "StartPrizeDraw: Unable to fetch prize message info, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	//fetch code
	var rawCode string
	err = config.DB.QueryRow(ctx, "select pgp_sym_decrypt(code::bytea,$2) as code from codes where id=$1", selectedEntry.Code.Id, config.EncryptionKey).
		Scan(&rawCode)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "StartPrizeDraw: Unable to get selected code info, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	tx, err := config.DB.Begin(ctx)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "StartPrizeDraw: Unable to begin transaction query, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	defer func() {
		if err != nil {
			tx.Rollback(ctx)
		}
	}()
	//insert draw
	var drawId int
	err = tx.QueryRow(ctx, "insert into draw (prize_type_id,entry_id,code,customer_id,status,operator_id) values ($1,$2,$3,$4,$5,$6) returning id",
		formData.PrizeType, selectedEntry.Id, rawCode, selectedEntry.Customer.Id, "confirmed", userPayload.Id).
		Scan(&drawId)
	if err != nil {
		if ok, keyy := utils.IsErrDuplicate(err); ok {
			if keyy == "unique_customer_prize" {
				return utils.JsonErrorResponse(c, fiber.StatusExpectationFailed, "Unable to confirm the draw, The same customer has already won a prize")
			}
			return utils.JsonErrorResponse(c, fiber.StatusExpectationFailed, "The selected entry has already won a prize")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "StartPrizeDraw: Unable to save draw data, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	//insert prize
	var prizeId int
	err = tx.QueryRow(ctx, "insert into prize (entry_id,prize_type_id,prize_value,code,draw_id) values ($1,$2,$3,$4,$5) returning id",
		selectedEntry.Id, formData.PrizeType, value, rawCode, drawId).
		Scan(&prizeId)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "StartPrizeDraw: Unable to save prize data, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	//distribute prize
	if distributionType == "momo" {
		_, err = tx.Exec(ctx, `insert into transaction (prize_id, amount, phone, mno, customer_id, transaction_type, initiated_by,status) values ($1, $2, $3, $4, $5,'CREDIT','SYSTEM','WAITING')`,
			prizeId, value, customerPhone, mno, selectedEntry.Customer.Id)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start a new draw, system error", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "entrySaveCode: #distribute_prize insert transaction failed: err:" + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	tx.Commit(ctx)
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "startPrizeDraw",
			Description:  "Started a new draw, winner: " + customerName + ", code: " + rawCode,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"draw_id":     drawId,
			"prize_id":    prizeId,
			"customer_id": selectedEntry.Customer.Id,
		},
	)
	//send sms
	go utils.SendSMS(config.DB, customerPhone, prizeMessage, viper.GetString("SENDER_ID"), config.ServiceName, "prize_won", &selectedEntry.Customer.Id, config.Redis)
	c.SendStatus(200)
	arrayCode := strings.Split(rawCode, "")
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "Draw ended successfully",
		"winner": fiber.Map{"draw_id": drawId, "prize_id": prizeId, "winner": customerName, "code": arrayCode},
	})
}

func GetDistributionType(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	types := strings.Split(viper.GetString("DISTRIBUTION_TYPES"), ",")
	distributions := []map[string]string{}
	for _, t := range types {
		distributions = append(distributions, map[string]string{"name": t, "id": t})
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": distributions})
}
func GetDepartments(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	departments := []model.Department{}
	rows, err := config.DB.Query(ctx,
		`select id,title,created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' from departments`)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get department data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetDepartments: Unable to get department data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "department data is not valid")
	}
	for rows.Next() {
		department := model.Department{}
		err = rows.Scan(&department.Id, &department.Title, &department.CreatedAt)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get department data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetDepartments: Unable to get department data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		departments = append(departments, department)
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": departments})
}
func GetSMSSent(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type SmsData struct {
		Id           string    `json:"id"`
		Message      string    `json:"message"`
		MessageType  string    `json:"message_type"`
		Phone        *string   `json:"phone"`
		Status       string    `json:"status"`
		ErrorMessage string    `json:"error_message"`
		CreatedAt    time.Time `json:"created_at"`
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	messageType := c.Query("message_type")
	//add pagination
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	offSet := (page - 1) * limit

	err = utils.ValidateDateRanges(startDateStr, &endDateStr)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, err.Error())
	}

	args1 := []interface{}{}
	// limitStr := fmt.Sprintf(" limit $%d offset $%d", a+1, a+2)
	logsFilter, ii := utils.BuildQueryFilter(
		map[string]interface{}{
			"sms.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= ": startDateStr,
			"sms.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= ": endDateStr,
			"sms.type": messageType,
		},
		&args1,
	)
	limitStr := fmt.Sprintf(" limit $%d offset $%d", ii, ii+1)
	globalArgs := args1
	args1 = append(args1, limit, offSet)
	smsData := []SmsData{}
	rows, err := config.DB.Query(ctx,
		`select message_id,message,phone,type,status,error_message,created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' from sms`+logsFilter+` order by id desc`+limitStr, args1...)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get sms data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetSMSSent: Unable to get sms data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "sms data is not valid")
	}
	for rows.Next() {
		sms := SmsData{}
		err = rows.Scan(&sms.Id, &sms.Message, &sms.Phone, &sms.MessageType, &sms.Status, &sms.ErrorMessage, &sms.CreatedAt)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get sms data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetSMSSent: Unable to get sms data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		smsData = append(smsData, sms)
	}
	totalSms := 0
	err = config.DB.QueryRow(ctx,
		`select count(id) from sms `+logsFilter, globalArgs...).Scan(&totalSms)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get sms sent data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetSMSSent: Unable to get total sms sent data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": smsData,
		"pagination": fiber.Map{"page": page, "limit": limit, "total": totalSms}})
}
func GetPrizeOverview(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type PrizeOverview struct {
		TotalPrize      float64 `json:"total_prize"`
		PrizeCount      int     `json:"prize_count"`
		TotalEligibilty float64 `json:"total_elligibility"`
		PrizeType       string  `json:"prize_type"`
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	if startDateStr == "" {
		//set default to today
		startDateStr = time.Now().Format("2006-01-02")
	}
	if endDateStr == "" {
		//set default to today
		startDateStr = time.Now().Format("2006-01-02")
	}
	dateFilter := ""
	dateFilterEntry := ""
	args := []interface{}{}
	var startDate time.Time
	if len(startDateStr) != 0 {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid start date provided")
		}
		dateFilter += "p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= $1"
		dateFilterEntry += "e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= $1"
		args = append(args, startDateStr)
	}
	if len(endDateStr) != 0 {
		endDate, err := time.Parse("2006-01-02", endDateStr)
		//check if end date is after start date
		if len(startDateStr) != 0 {
			if endDate.Before(startDate) {
				return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "End date should be after start date")
			}
		}
		//add one day to include the end date
		endDate = endDate.AddDate(0, 0, 1)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid end date provided")
		}
		argName := "$1"
		if len(dateFilter) != 0 {
			dateFilter += " and "
			dateFilterEntry += " and "
			argName = "$2"
		}
		args = append(args, endDate)
		dateFilter += "p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= " + argName
		dateFilterEntry += "e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= " + argName
	}
	if len(dateFilter) != 0 {
		dateFilter = " where " + dateFilter
		dateFilterEntry = " where " + dateFilterEntry
	}
	prizeOverviews := []PrizeOverview{}
	query := fmt.Sprintf(`select sum(p.prize_value),count(p.id),pt.elligibility,pt.name from prize p
	INNER JOIN prize_type pt ON pt.id=p.prize_type_id %s group by p.prize_type_id,pt.id`, dateFilter)
	rows, err := config.DB.Query(ctx, query, args...)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prizeOverview data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeOverview: Unable to get prizeOverview data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "prizeOverview data is not valid")
	}
	for rows.Next() {
		prizeOverview := PrizeOverview{}
		err = rows.Scan(&prizeOverview.TotalPrize, &prizeOverview.PrizeCount, &prizeOverview.TotalEligibilty, &prizeOverview.PrizeType)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prizeOverview data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeOverview: Unable to get prizeOverview data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		prizeOverviews = append(prizeOverviews, prizeOverview)
	}
	//get total end based on date range
	totalEntries := 0
	queryEntry := fmt.Sprintf(`select count(id) from entries e %s`, dateFilterEntry)
	err = config.DB.QueryRow(ctx, queryEntry, args...).Scan(&totalEntries)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prizeOverview data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeOverview: Unable to get entry summary, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "prize_overview": prizeOverviews, "total_entries": totalEntries})
}

func GetCodeOverview(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type CodeOverview struct {
		TotalCode  int `json:"totalCode"`
		UsedCode   int `json:"usedCode"`
		RemainCode int `json:"remainCode"`
	}
	codeOverview := CodeOverview{}
	//pending_count
	err = config.DB.QueryRow(ctx, `SELECT total,used_count FROM codes_count;`).Scan(&codeOverview.TotalCode, &codeOverview.UsedCode)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get codeOverview data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetCodeOverview: Unable to get codeOverview data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		codeOverview = CodeOverview{
			TotalCode:  0,
			UsedCode:   0,
			RemainCode: 0,
		}
	}
	codeOverview.RemainCode = codeOverview.TotalCode - codeOverview.UsedCode
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": codeOverview})
}

// function to upload excel file and insert into codes table after validation, and use transaction to rollback if any error occurs
func UploadCodes(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	if !userPayload.CanAddCodes {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "You don't have permission to upload codes")
	}
	startTime := time.Now()
	fmt.Println("Upload started at:", startTime.Format("2006-01-02 15:04:05"))
	//TODO: check if user has right to upload code
	file, err := c.FormFile("file")
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusBadRequest, "Please provide a valid file")
	}
	if file.Size > 1024*1024*100 {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "File size should not exceed 100MB")
	}
	//save file
	fileName := fmt.Sprintf("/app/uploads/%s", file.Filename)
	err = c.SaveFile(file, fileName)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to save file", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "UploadCodes: Unable to save file, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	var codes []model.Code
	//postgres limit max value to 65535, make sure to split the data into batches
	var batches [][]model.Code
	ext := filepath.Ext(file.Filename)
	bCount := 0
	skippedLines := 0
	if ext == ".txt" {
		//read file content and split by new line
		// content, err := os.ReadFile(fileName)
		// if err != nil {
		// 	return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to read file", utils.Logger{
		// 		LogLevel:    utils.CRITICAL,
		// 		Message:     "UploadCodes: Unable to read file, error: " + err.Error(),
		// 		ServiceName: config.ServiceName,
		// 	})
		// }
		// lines := strings.Split(string(content), "\n")
		// for _, line := range lines {
		// 	//limit batch to 20000 because each value contains 3 params
		// 	if bCount%20000 == 0 && bCount != 0 {
		// 		batches = append(batches, codes)
		// 		codes = []model.Code{}
		// 	}
		// 	bCount++
		// 	//prevent reading more than 3 empty lines
		// 	skippedLines++
		// 	if skippedLines > 3 {
		// 		break
		// 	}
		// 	line = strings.TrimSpace(line)
		// 	if len(line) == 10 {
		// 		codes = append(codes, model.Code{Code: line})
		// 	} else {
		// 		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid code length, code should be 10 digits. code: "+line)
		// 	}
		// }

		nFile, err := os.Open(fileName)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to read file", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "UploadCodes: Unable to read file, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		defer nFile.Close()
		scanner := bufio.NewScanner(nFile)
		for scanner.Scan() {
			//limit batch to 20000 because each value contains 3 params
			if bCount%20000 == 0 && bCount != 0 {
				batches = append(batches, codes)
				codes = []model.Code{}
			}
			//prevent reading more than 3 empty lines
			if skippedLines > 3 {
				break
			}
			line := scanner.Text()
			line = strings.TrimSpace(line)
			if line == "" {
				skippedLines++
				continue // skip empty lines
			}
			bCount++
			if len(line) == 10 {
				codes = append(codes, model.Code{Code: line})
			} else {
				return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid code length, code should be 10 digits. code: "+line)
			}
		}
		if len(codes) > 0 {
			batches = append(batches, codes)
		}
		fmt.Println("Batch preparing completed:", bCount, ", at: ", time.Now().Format("2006-01-02 15:04:05"), ", Took time:", time.Since(startTime))
	} else if ext == ".xlsx" || ext == ".xls" {
		//open file
		xlFile, err := excelize.OpenFile(fileName)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to open file", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "UploadCodes: Unable to open file, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		rows, err := xlFile.GetRows("Sheet1")
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to read file", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "UploadCodes: Unable to read file, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		//validate file data

		for i, row := range rows {
			if i == 0 {
				continue
			}
			if len(row) != 1 {
				return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid file format, each row should contain only one code")
			}
			if bCount%20000 == 0 && bCount != 0 {
				batches = append(batches, codes)
				codes = []model.Code{}
			}
			if len(row[0]) == 0 {
				skippedLines++
				continue
			}
			bCount++
			if skippedLines > 3 {
				break
			}
			line := strings.TrimSpace(row[0])
			if len(line) == 10 {
				codes = append(codes, model.Code{Code: line})
			} else {
				return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid code length, code should be 10 digits. code: "+line)
			}
		}
		if len(codes) > 0 {
			batches = append(batches, codes)
		}
		fmt.Println("Batch preparing using excel file completed:", bCount, ", at: ", time.Now().Format("2006-01-02 15:04:05"), ", Took time:", time.Since(startTime))
	} else {
		return c.Status(fiber.StatusUnsupportedMediaType).SendString("Unsupported file type")
	}
	//run upload process in background and notify user via SMS
	ipAddress := c.IP()
	webAgent := c.Get("User-Agent")
	go func() {
		//insert codes
		tx, err := config.DB.Begin(ctx)
		if err != nil {
			//send sms notification on error
			traceId := utils.LogMessage("error", "UploadCodes: Unable to start transaction, error: "+err.Error(), config.ServiceName)
			go utils.SendSMS(config.DB, userPayload.Phone, "Codes uploaded failed, Unable to start transaction, error code: "+traceId,
				viper.GetString("SENDER_ID"), config.ServiceName, "upload_codes", nil, config.Redis)
			return
		}
		defer func() {
			if err != nil {
				tx.Rollback(ctx)
			} else {
				tx.Commit(ctx)
			}
		}()
		rowsAffected := 0
		for _, batch := range batches {
			// Prepare the values and placeholders
			valueStrings := make([]string, 0, len(batch))
			valueArgs := make([]interface{}, 0, len(batch)*3)
			for _, code := range batch {
				code.Code = strings.ToUpper(code.Code)
				valueStrings = append(valueStrings, fmt.Sprintf("(pgp_sym_encrypt($%d, $%d), digest($%d, 'sha256'), 'unused')",
					len(valueArgs)+1,
					len(valueArgs)+2,
					len(valueArgs)+1))

				valueArgs = append(valueArgs, code.Code, config.EncryptionKey)
			}
			// Construct the full SQL query
			query := fmt.Sprintf("INSERT INTO codes (code, code_hash, status) VALUES %s",
				strings.Join(valueStrings, ", "))
			// Execute the multi-value insert
			cmdTag, err := tx.Exec(context.Background(), query, valueArgs...)
			if err != nil {
				if ok, _ := utils.IsErrDuplicate(err); ok {
					err = errors.New("some codes already exist")
				} else {
					traceId := utils.LogMessage("error", "UploadCodes: Unable to insert codes, error: "+err.Error(), config.ServiceName)
					err = errors.New("Unable to insert codes, error code: " + traceId)
				}
			}
			if err != nil {
				//send sms notification on error
				go utils.SendSMS(config.DB, userPayload.Phone, fmt.Sprintf("Codes uploaded failed, %s", err.Error()),
					viper.GetString("SENDER_ID"), config.ServiceName, "upload_codes", nil, config.Redis)
				return
			}
			rowsAffected += int(cmdTag.RowsAffected())
			fmt.Println("Batch item completed affected rows:", rowsAffected, ", at: ", time.Now().Format("2006-01-02 15:04:05"))
		}
		_, err = config.DB.Exec(ctx, "REFRESH MATERIALIZED VIEW codes_count;")
		if err != nil {
			utils.LogMessage("error", "UploadCodes: Unable to refresh codes_count, error: "+err.Error(), config.ServiceName)
			err = nil
		}
		fmt.Println("Upload completed, ", "Total rows:", bCount, "affected rows:", rowsAffected, ", at: ", time.Now().Format("2006-01-02 15:04:05"), ", Took time:", time.Since(startTime))
		//send sms notification
		go utils.SendSMS(config.DB, userPayload.Phone, fmt.Sprintf("Codes uploaded successfully, total: %d, time taken: %v", rowsAffected, time.Since(startTime)), viper.GetString("SENDER_ID"), config.ServiceName, "upload_codes", nil, config.Redis)
		utils.RecordActivityLog(config.DB,
			utils.ActivityLog{
				UserID:       userPayload.Id,
				ActivityType: "uploadCodes",
				Description:  "Upload codes, total: " + fmt.Sprintf("%v", rowsAffected),
				Status:       "success",
				IPAddress:    ipAddress,
				UserAgent:    webAgent,
			},
			config.ServiceName,
			nil,
		)
		//remove file
		go os.Remove(fileName)
	}()
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": fmt.Sprintf("%v Codes will be uploaded in background and we will send you an SMS", bCount), "count": bCount})
}
func GetLogs(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	if !userPayload.CanViewLogs {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "You don't have permission to view logs")
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	userId := c.Query("user_id")
	query := c.Query("query")
	//add pagination
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	offSet := (page - 1) * limit

	err = utils.ValidateDateRanges(startDateStr, &endDateStr)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, err.Error())
	}
	if len(userId) != 0 {
		//check if user id is valid integer
		_, err := strconv.Atoi(userId)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid user id provided")
		}
	}

	args1 := []interface{}{}
	// limitStr := fmt.Sprintf(" limit $%d offset $%d", a+1, a+2)
	logsFilter, ii := utils.BuildQueryFilter(map[string]interface{}{
		"l.created_at AT TIME ZONE 'Africa/Kigali' >= ": startDateStr,
		"l.created_at AT TIME ZONE 'Africa/Kigali' <= ": endDateStr,
		"l.user_id": userId,
	},
		&args1,
	)
	if len(query) != 0 {
		//check if user id is valid integer
		if utils.ValidateString(query, "") {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Query contains invalid characters")
		}
		if len(logsFilter) != 0 {
			logsFilter += " and "
		} else {
			logsFilter += " where "
		}
		args1 = append(args1, query)
		// logsFilter += "l.description ilike " + fmt.Sprintf("$%d", a)
		logsFilter += "to_tsvector('english', l.description) @@ plainto_tsquery('english', " + fmt.Sprintf("$%d", ii) + ")"
		ii++
	}
	limitStr := fmt.Sprintf(" limit $%d offset $%d", ii, ii+1)
	globalArgs := args1
	args1 = append(args1, limit, offSet)
	logs := []utils.ActivityLog{}
	rows, err := config.DB.Query(ctx,
		`select l.user_id,l.activity_type,l.status,l.description,l.ip_address::text,l.user_agent,l.created_at AT TIME ZONE 'Africa/Kigali',u.fname,u.lname,u.email,u.phone from activity_logs l `+
			` inner join users u on u.id = l.user_id `+logsFilter+` order by l.created_at AT TIME ZONE 'Africa/Kigali' desc`+limitStr, args1...)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get activity logs data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetLogs: Unable to get sms data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "activity logs data is not valid")
	}
	for rows.Next() {
		log := utils.ActivityLog{}
		err = rows.Scan(&log.UserID, &log.ActivityType, &log.Status, &log.Description, &log.IPAddress, &log.UserAgent, &log.CreatedAt,
			&log.User.Fname, &log.User.Lname, &log.User.Email, &log.User.Phone)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get activity logs data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetLogs: Unable to get activity logs data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		logs = append(logs, log)
	}
	//get total logs for pagination
	//get total logs for pagination
	totalLogs := 0
	err = config.DB.QueryRow(ctx,
		`select count(id) from activity_logs l `+logsFilter, globalArgs...).Scan(&totalLogs)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get activity logs data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetLogs: Unable to get total logs data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": logs,
		"pagination": fiber.Map{"page": page, "limit": limit, "total": totalLogs}})
}

// TODO: implement loop to distribute prize to users
func DistributeMomoPrize() {
	//fetch pending transaction
	// fmt.Println("Distributing momo prize")
	//refresh codes after 20 min
	if time.Now().Minute()%20 == 0 {
		_, err := config.DB.Exec(ctx, "REFRESH MATERIALIZED VIEW codes_count;")
		if err != nil {
			utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to refresh codes_count, error: "+err.Error(), config.ServiceName)
		}
	}
	rows, err := config.DB.Query(ctx, `SELECT t.id,t.amount,t.phone,t.mno,t.trx_id,t.transaction_type,p.code FROM transaction t
	INNER JOIN prize p on p.id = t.prize_id WHERE t.status = 'PENDING' LIMIT 1000;`)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to fetch pending momo transactions, error: "+err.Error(), config.ServiceName)
		}
	}
	a := 0
	for rows.Next() {
		a++
		transaction := model.Transactions{}
		err = rows.Scan(&transaction.Id, &transaction.Amount, &transaction.Phone, &transaction.Mno, &transaction.TrxId, &transaction.TransactionType, &transaction.Code)
		if err != nil {
			utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to scan pending momo transactions, error: "+err.Error(), config.ServiceName)
			continue
		}
		//check first if there is no transaction_records with status=SUCCESS
		var trxRecordId int
		err = config.DB.QueryRow(ctx, `select id from transaction_records where transaction_id=$1 and status='SUCCESS'`, transaction.Id).Scan(&trxRecordId)
		if err == nil {
			//update transaction status to FAILED
			_, err = config.DB.Exec(ctx, `UPDATE transaction SET status = 'SUCCESS', error_message = 'Duplicate transaction' WHERE id = $1;`, transaction.Id)
			if err != nil {
				utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to update momo transaction status, error: "+err.Error(), config.ServiceName)
			}
			continue
		}
		refNo := ""
		//create new transaction_record
		var trxRecordCount int
		newTrxid := transaction.TrxId
		err = config.DB.QueryRow(ctx, `select count(id) from transaction_records where transaction_id=$1`, transaction.Id).Scan(&trxRecordCount)
		if err != nil {
			utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to fetch transaction_records count, error: "+err.Error(), config.ServiceName)
		}
		previousTrxId := newTrxid
		if trxRecordCount == 1 {
			newTrxid = newTrxid + "R1"
		} else if trxRecordCount > 1 {
			// previousTrxId = fmt.Sprintf("%s%d", newTrxid[:len(newTrxid)-1], (trxRecordCount - 1))
			previousTrxId = fmt.Sprintf("%sR%d", newTrxid, (trxRecordCount - 1))
			newTrxid = fmt.Sprintf("%sR%d", newTrxid, trxRecordCount)
		}
		fmt.Println("New trxId: ", newTrxid)
		fmt.Println("Previous trxId: ", previousTrxId)
		//check  status for previous transaction
		var statusErr error
		var statusRefNo string
		if transaction.Mno == "MTN" {
			statusRefNo, statusErr = utils.MoMoCheckStatus(previousTrxId)

		} else if transaction.Mno == "AIRTEL" {
			statusRefNo, statusErr = utils.AirtelCheckStatus(previousTrxId, *config.Redis)
		}
		if statusErr == nil {
			//transaction already completed
			_, err = config.DB.Exec(ctx, `UPDATE transaction SET status = 'SUCCESS', ref_no = $1 WHERE id = $2;`, statusRefNo, transaction.Id)
			if err != nil {
				utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to update momo transaction status, error: "+err.Error(), config.ServiceName)
			}
			utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to check  transaction status already success, newTrxid: "+newTrxid+", previousTrxId:"+previousTrxId, config.ServiceName)
			continue
		} else {
			utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to check momo transaction status, error: "+statusErr.Error(), config.ServiceName)
		}
		var newTrxRecordId int
		err := config.DB.QueryRow(ctx, `insert into transaction_records (transaction_id, trx_id,amount,phone,transaction_type,mno,status) values ($1, $2, $3, $4, $5, $6, 'PENDING') returning id`,
			transaction.Id, newTrxid, transaction.Amount, transaction.Phone, "CREDIT", transaction.Mno).Scan(&newTrxRecordId)
		if err != nil {
			utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to create new transaction_record, error: "+err.Error(), config.ServiceName)
			continue
		}
		if transaction.Mno == "MTN" {
			refNo, err = utils.MoMoCredit(transaction.Amount, transaction.Phone, newTrxid, transaction.Code)
		} else if transaction.Mno == "AIRTEL" {
			refNo, err = utils.AirtelCredit(transaction.Amount, transaction.Phone, newTrxid, transaction.Code, *config.Redis)
		} else {
			err = errors.New("invalid network operator")
		}
		//TODO: add other network operators (Airtel)
		if err != nil {
			//update transaction status and error_message
			err1 := err.Error()
			_, err = config.DB.Exec(ctx, `UPDATE transaction SET status = 'FAILED', error_message = $1 WHERE id = $2;`, err1, transaction.Id)
			if err != nil {
				utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to update momo transaction status, error: "+err.Error(), config.ServiceName)
			}
			_, err = config.DB.Exec(ctx, `UPDATE transaction_records SET status = 'FAILED', error_message = $1 WHERE id = $2;`, err1, newTrxRecordId)
			if err != nil {
				utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to update FAILED momo transaction_records status, error: "+err.Error(), config.ServiceName)
			}
		} else {
			//update transaction status and ref_no
			_, err = config.DB.Exec(ctx, `UPDATE transaction SET status = 'SUCCESS', ref_no = $1 WHERE id = $2;`, refNo, transaction.Id)
			if err != nil {
				utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to update momo transaction status, error: "+err.Error(), config.ServiceName)
			}
			_, err = config.DB.Exec(ctx, `UPDATE transaction_records SET status = 'SUCCESS', ref_no = $1 WHERE id = $2;`, refNo, newTrxRecordId)
			if err != nil {
				utils.LogMessage(string(utils.CRITICAL), "DistributeMomoPrize: Unable to update SUCCESS momo transaction_records status, error: "+err.Error(), config.ServiceName)
			}
		}
	}
	// fmt.Println("Distributing momo prize end, rows affected: ", a)
	//re run the function after 60 seconds
	time.Sleep(60 * time.Second)
	DistributeMomoPrize()
}
func GetSMSBalance(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	smsBalance, err := utils.SMSBalance(config.DB, config.ServiceName, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to get sms balance", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "GetSMSBalance: Unable to get sms balance, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "balance": smsBalance})
}
func ChangeUserStatus(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	if !userPayload.CanAddUser {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "You don't have permission to update user info")
	}
	userId, err := c.ParamsInt("userId")
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid user id provided")
	}
	type FormData struct {
		Status string `json:"status" binding:"required" validate:"required,oneof=OKAY DISABLED"`
	}
	responseStatus := 200
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil || formData.Status == "" {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data - " + formData.Status, "details": err})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provide data are not valid")
	}
	//get existing status
	var status, fname, phone string
	err = config.DB.QueryRow(ctx, `select status,fname,phone from users where id=$1`, userId).Scan(&status, &fname, &phone)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "User data is invalid")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to change user status", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "ChangeUserStatus: Unable to change user status, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	if status == formData.Status {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "User status is already "+formData.Status)
	}
	_, err = config.DB.Exec(ctx, `update users set status=$1 where id=$2`, formData.Status, userId)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to change user status", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "ChangeUserStatus: Unable to change user status, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	action := "enable"
	if formData.Status == "DISABLED" {
		action = "disable"
	}
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: action + "User",
			Description:  action + " user account of" + fname + ", Phone: " + phone,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"user_id": userId,
			"status":  formData.Status,
		},
	)
	return c.JSON(fiber.Map{"status": responseStatus, "message": "Account of " + fname + " " + action + "d successfully"})
}
func GetProvinces(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	provinces := []model.Province{}
	rows, err := config.DB.Query(ctx,
		`select id,name,created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' from province`)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get province data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetProvinces: Unable to get province data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "province data is not valid")
	}
	for rows.Next() {
		province := model.Province{}
		err = rows.Scan(&province.Id, &province.Name, &province.CreatedAt)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get province data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetProvinces: Unable to get province data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		provinces = append(provinces, province)
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": provinces})
}
func GetTransactions(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	phone := c.Query("phone")
	trxId := c.Query("trx_id")
	refNo := c.Query("ref_no")
	status := c.Query("status")
	//(optional), excel/pdf. if it is set system will not implement pagination
	export := c.Query("export")
	//add pagination
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	offSet := (page - 1) * limit
	if status == "COMPLETED" {
		status = "SUCCESS"
	}
	err = utils.ValidateDateRanges(startDateStr, &endDateStr)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, err.Error())
	}
	// validate the phone using this regex ^2507[2389]\\d{7}$
	regex := regexp.MustCompile(`^2507[2389]\d{7}$`)
	if len(phone) != 0 {
		if !regex.MatchString(phone) {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid phone number provided")
		}
	}
	args1 := []interface{}{}
	// limitStr := fmt.Sprintf(" limit $%d offset $%d", a+1, a+2)
	logsFilter, ii := utils.BuildQueryFilter(
		map[string]interface{}{
			"t.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= ": startDateStr,
			"t.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= ": endDateStr,
			"t.phone":  phone,
			"t.ref_no": refNo,
			"t.trx_id": trxId,
			"t.status": status,
		},
		&args1,
	)
	limitStr := ""
	globalArgs := args1
	if export == "" {
		limitStr = fmt.Sprintf(" limit $%d offset $%d", ii, ii+1)
		args1 = append(args1, limit, offSet)
	}
	transactions := []model.Transactions{}
	rows, err := config.DB.Query(ctx,
		`select t.id,t.amount,t.phone,t.mno,coalesce(tr.trx_id,t.trx_id) as trx_id,t.ref_no,t.transaction_type,t.status,CASE WHEN t.status='SUCCESS' THEN '' ELSE t.error_message END as error_message,
		t.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali',p.entry_id,p.code,t.customer_id,t.initiated_by,t.updated_at,p.id as prize_id,pt.name from transaction t
		inner join prize p on p.id = t.prize_id LEFT JOIN prize_type pt ON p.prize_type_id=pt.id LEFT JOIN (select max(id) as id,max(trx_id) as trx_id,transaction_id from transaction_records group by transaction_id) tr ON tr.transaction_id = t.id `+logsFilter+` order by t.created_at desc`+limitStr, args1...)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get transaction data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetTransactions: Unable to get sms data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "transaction data is not valid")
	}
	for rows.Next() {
		transaction := model.Transactions{}
		err = rows.Scan(&transaction.Id, &transaction.Amount, &transaction.Phone, &transaction.Mno, &transaction.TrxId, &transaction.RefNo, &transaction.TransactionType,
			&transaction.Status, &transaction.ErrorMessage, &transaction.CreatedAt, &transaction.EntryId, &transaction.Code, &transaction.CustomerId,
			&transaction.InitiatedBy, &transaction.UpdatedAt, &transaction.PrizeId, &transaction.PrizeType)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get transaction data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetTransactions: Unable to get transaction data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		transaction.Charges = 120
		transactions = append(transactions, transaction)
	}
	//get total logs for pagination
	totalTransaction := 0
	err = config.DB.QueryRow(ctx,
		`select count(id) from transaction t `+logsFilter, globalArgs...).Scan(&totalTransaction)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get transaction data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetTransactions: Unable to get total transaction data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	//export data to excel
	if export == "excel" {
		fileName := fmt.Sprintf("transactions_%s.xlsx", time.Now().Format("2006-01-02_15-04-05"))
		filePath := fmt.Sprintf("%s/%s", viper.GetString("EXPORT_PATH"), fileName)
		rawData, err := utils.ExportToExcel(filePath, "Transactions", transactions)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to export transactions to excel", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetTransactions: Unable to export transactions to excel, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
		return c.Send(rawData)
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": transactions,
		"pagination": fiber.Map{"page": page, "limit": limit, "total": totalTransaction}})
}
func ConfirmTransaction(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	transactionId, err := c.ParamsInt("transaction_id")
	if err != nil || transactionId < 1 {
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "Invalid transaction id provided")
	}
	responseStatus := 200
	type FormData struct {
		Password string `json:"password" binding:"required" validate:"required"`
	}
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data", "details": err})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provided data are not valid")
	}
	//check if password is correct
	var fname, lname, userStatus string
	err = config.DB.QueryRow(ctx,
		`select u.fname,u.lname,u.status from users u  where id = $1 and password = crypt($2, password)`, userPayload.Id, formData.Password).
		Scan(&fname, &lname, &userStatus)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			utils.LogMessage("critical", fmt.Sprintf("ConfirmTransaction: Unable to get user data for password verify, Email:%s, err:%v", userPayload.Email, err), "web-service")
		}
		responseStatus = 401
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Invalid credentials"})
	} else if userStatus != "OKAY" {
		responseStatus = 401
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Your account has been deactivated"})
	}
	//get existing status
	var status, phone, trxId, prizeCode string
	var refNo *string
	var amount float64
	err = config.DB.QueryRow(ctx, `select status,phone,trx_id,ref_no,amount,code from transaction t
	INNER JOIN prize p on p.id = t.prize_id where t.id=$1`, transactionId).Scan(&status, &phone, &trxId, &refNo, &amount, &prizeCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Transaction data is invalid")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to confirm transaction", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "ConfirmTransaction: Unable to confirm transaction, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	if status != "WAITING" {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Transaction status is already confirmed, refresh your page")
	}
	_, err = config.DB.Exec(ctx, `update transaction set status=$1 where id=$2`, "PENDING", transactionId)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to confirm transaction", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "ConfirmTransaction: Unable to confirm transaction, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "confirmTransaction",
			Description:  "confirm transaction of" + phone + ", TrxId: " + trxId + ", Prize: " + prizeCode,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"transaction_id": transactionId,
			"status":         "WAITING",
		},
	)
	return c.JSON(fiber.Map{"status": responseStatus, "message": fmt.Sprintf("Transaction of %s confirmed successfully", prizeCode)})
}

// confirmBulkTransaction which has WAITING status to PENDING
func ConfirmBulkTransaction(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type FormData struct {
		TransactionIds []int  `json:"transaction_ids" binding:"required" validate:"required"`
		Password       string `json:"password" binding:"required" validate:"required"`
	}
	responseStatus := 200
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil || len(formData.TransactionIds) == 0 {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data", "details": err})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provided data are not valid")
	}
	//check if password is correct
	var fname, lname, userStatus string
	err = config.DB.QueryRow(ctx,
		`select u.fname,u.lname,u.status from users u  where id = $1 and password = crypt($2, password)`, userPayload.Id, formData.Password).
		Scan(&fname, &lname, &userStatus)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			utils.LogMessage("critical", fmt.Sprintf("ConfirmBulkTransaction: Unable to get user data for password verify, Email:%s, err:%v", userPayload.Email, err), "web-service")
		}
		responseStatus = 401
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Invalid credentials"})
	} else if userStatus != "OKAY" {
		responseStatus = 401
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Your account has been deactivated"})
	}
	//get existing status
	prizeCodes := []string{}
	var status, phone, trxId, prizeCode string
	var refNo *string
	var amount float64
	for _, transactionId := range formData.TransactionIds {
		err = config.DB.QueryRow(ctx, `select status,phone,trx_id,ref_no,amount,code from transaction t
		INNER JOIN prize p on p.id = t.prize_id where t.id=$1`, transactionId).Scan(&status, &phone, &trxId, &refNo, &amount, &prizeCode)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Transaction data is invalid")
			}
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to confirm transaction", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "ConfirmBulkTransaction: Unable to confirm transaction, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		prizeCodes = append(prizeCodes, prizeCode)
		if status != "WAITING" {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "One of the transaction status is already confirmed, refresh your page #"+prizeCode)
		}
	}
	tx, err := config.DB.Begin(ctx)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start transactions confirmation", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "ConfirmBulkTransaction: Unable to start transaction confirmation, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	defer func() {
		if err != nil {
			tx.Rollback(ctx)
		} else {
			tx.Commit(ctx)
		}
	}()
	for _, transaction := range formData.TransactionIds {
		_, err = tx.Exec(ctx, `update transaction set status=$1 where id=$2`, "PENDING", transaction)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to confirm transaction", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "ConfirmBulkTransaction: Unable to confirm transaction, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "confirmBulkTransaction",
			Description:  fmt.Sprintf("confirm bulk transaction, Count:%d, Codes: %s", len(prizeCodes), strings.Join(prizeCodes, ",")),
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"transaction_ids": formData.TransactionIds,
			"status":          "PENDING",
		},
	)
	return c.JSON(fiber.Map{"status": responseStatus, "message": fmt.Sprintf("Transaction of %s confirmed successfully", prizeCode)})
}

func TestSMS(c *fiber.Ctx) error {
	//get all sms
	phone := c.Params("phone")
	messageId, err := utils.SendSMS(config.DB, phone, "Hey, this is a test message from SMPP MTN", viper.GetString("SENDER_ID"), config.ServiceName, "test_sms", nil, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to send sms", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "TestSMS: Unable to send sms, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "SMS sent", "message_id": messageId})
}
func ResendTransaction(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	transactionId, err := c.ParamsInt("transaction_id")
	if err != nil || transactionId < 1 {
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "Invalid transaction id provided")
	}
	responseStatus := 200
	type FormData struct {
		Password string `json:"password" binding:"required" validate:"required"`
	}
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data", "details": err})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provided data are not valid")
	}
	//check if password is correct
	var fname, lname, userStatus string
	err = config.DB.QueryRow(ctx,
		`select u.fname,u.lname,u.status from users u  where id = $1 and password = crypt($2, password)`, userPayload.Id, formData.Password).
		Scan(&fname, &lname, &userStatus)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			utils.LogMessage("critical", fmt.Sprintf("ConfirmTransaction: Unable to get user data for password verify, Email:%s, err:%v", userPayload.Email, err), "web-service")
		}
		responseStatus = 401
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Invalid credentials"})
	} else if userStatus != "OKAY" {
		responseStatus = 401
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Your account has been deactivated"})
	}
	//get existing status
	var status, phone, trxId, prizeCode string
	var refNo *string
	var amount float64
	err = config.DB.QueryRow(ctx, `select status,phone,trx_id,ref_no,amount,code from transaction t
	INNER JOIN prize p on p.id = t.prize_id where t.id=$1`, transactionId).Scan(&status, &phone, &trxId, &refNo, &amount, &prizeCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Transaction data is invalid")
		}
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to confirm transaction", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "ResendTransaction: Unable to confirm transaction, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	if status != "FAILED" {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Transaction status is already confirmed, refresh your page")
	}
	_, err = config.DB.Exec(ctx, `update transaction set status=$1 where id=$2`, "PENDING", transactionId)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to confirm transaction", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "ResendTransaction: Unable to confirm transaction, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "ResendTransaction",
			Description:  "Resend failed transaction of" + phone + ", TrxId: " + trxId + ", Prize: " + prizeCode,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"transaction_id": transactionId,
			"status":         "PENDING",
		},
	)
	return c.JSON(fiber.Map{"status": responseStatus, "message": fmt.Sprintf("Transaction of %s confirmed successfully", prizeCode)})
}
func ResendBulkTransaction(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type FormData struct {
		TransactionIds []int  `json:"transaction_ids" binding:"required" validate:"required"`
		Password       string `json:"password" binding:"required" validate:"required"`
	}
	responseStatus := 200
	formData := new(FormData)
	if err := c.BodyParser(formData); err != nil || len(formData.TransactionIds) == 0 {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data", "details": err})
	}
	if err := Validate.Struct(formData); err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Provided data are not valid")
	}
	//check if password is correct
	var fname, lname, userStatus string
	err = config.DB.QueryRow(ctx,
		`select u.fname,u.lname,u.status from users u  where id = $1 and password = crypt($2, password)`, userPayload.Id, formData.Password).
		Scan(&fname, &lname, &userStatus)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			utils.LogMessage("critical", fmt.Sprintf("ConfirmBulkTransaction: Unable to get user data for password verify, Email:%s, err:%v", userPayload.Email, err), "web-service")
		}
		responseStatus = 401
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Invalid credentials"})
	} else if userStatus != "OKAY" {
		responseStatus = 401
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Your account has been deactivated"})
	}
	//get existing status
	prizeCodes := []string{}
	var status, phone, trxId, prizeCode string
	var refNo *string
	var amount float64
	for _, transactionId := range formData.TransactionIds {
		err = config.DB.QueryRow(ctx, `select status,phone,trx_id,ref_no,amount,code from transaction t
		INNER JOIN prize p on p.id = t.prize_id where t.id=$1`, transactionId).Scan(&status, &phone, &trxId, &refNo, &amount, &prizeCode)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Transaction data is invalid")
			}
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to confirm transaction", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "ResendBulkTransaction: Unable to confirm transaction, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		prizeCodes = append(prizeCodes, prizeCode)
		if status != "FAILED" {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "One of the transaction status is already confirmed, refresh your page #"+prizeCode)
		}
	}
	tx, err := config.DB.Begin(ctx)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to start transactions confirmation", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     "ResendBulkTransaction: Unable to start transaction confirmation, error: " + err.Error(),
			ServiceName: config.ServiceName,
		})
	}
	defer func() {
		if err != nil {
			tx.Rollback(ctx)
		} else {
			tx.Commit(ctx)
		}
	}()
	for _, transaction := range formData.TransactionIds {
		_, err = tx.Exec(ctx, `update transaction set status=$1 where id=$2`, "PENDING", transaction)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to confirm transaction", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "ResendBulkTransaction: Unable to confirm transaction, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
	}
	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "ResendBulkTransaction",
			Description:  fmt.Sprintf("Resend failed bulk transaction, Count:%d, Codes: %s", len(prizeCodes), strings.Join(prizeCodes, ",")),
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		&map[string]interface{}{
			"transaction_ids": formData.TransactionIds,
			"status":          "PENDING",
		},
	)
	return c.JSON(fiber.Map{"status": responseStatus, "message": fmt.Sprintf("Transaction of %s confirmed successfully", prizeCode)})
}
func EditUser(c *fiber.Ctx) error {
	userPayload, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	if !userPayload.CanAddUser {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, "You don't have permission to update a user")
	}
	userId, err := c.ParamsInt("userId")
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid user id provided")
	}
	type FormData struct {
		Fname          string `json:"fname" binding:"required" validate:"required,regex=^[a-zA-Z0-9 ]*$"`
		Lname          string `json:"lname" binding:"required" validate:"required,regex=^[a-zA-Z0-9 ]*$"`
		Phone          string `json:"phone" binding:"required" validate:"required,regex=^2507[2389]\\d{7}$"`
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
	//fetch existing user data
	userData := model.UserProfile{}
	//fetch users
	err = config.DB.QueryRow(ctx,
		`select u.id,u.fname,u.lname,u.email,u.phone,u.department_id,d.title as department_title, u.email_verified,u.phone_verified,u.avatar_url,u.status,
			u.can_add_codes,u.can_trigger_draw,u.can_view_logs,u.can_add_user,force_change_password from users u inner join departments d on u.department_id = d.id where u.id=$1`, userId).
		Scan(&userData.Id, &userData.Fname, &userData.Lname, &userData.Email, &userData.Phone, &userData.Department.Id, &userData.Department.Title,
			&userData.EmailVerified, &userData.PhoneVerified, &userData.AvatarUrl, &userData.Status, &userData.CanAddCodes, &userData.CanTriggerDraw, &userData.CanViewLogs,
			&userData.CanAddUser, &userData.ForceChangePassword)
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
	logsChange := ""
	if userData.Fname != formData.Fname {
		logsChange += fmt.Sprintf("fname: %s -> %s, ", userData.Fname, formData.Fname)
	}
	if userData.Lname != formData.Lname {
		logsChange += fmt.Sprintf("lname: %s -> %s, ", userData.Lname, formData.Lname)
	}
	if userData.Phone != formData.Phone {
		logsChange += fmt.Sprintf("phone: %s -> %s, ", userData.Phone, formData.Phone)
	}
	if userData.Email != formData.Email {
		logsChange += fmt.Sprintf("email: %s -> %s, ", userData.Email, formData.Email)
	}
	if userData.Department.Id != formData.Department {
		logsChange += fmt.Sprintf("department: %d -> %d, ", userData.Department.Id, formData.Department)
	}
	if userData.CanAddCodes != formData.CanAddCode {
		logsChange += fmt.Sprintf("can_add_codes: %t -> %t, ", userData.CanAddCodes, formData.CanAddCode)
	}
	if userData.CanTriggerDraw != formData.CanTriggerDraw {
		logsChange += fmt.Sprintf("can_trigger_draw: %t -> %t, ", userData.CanTriggerDraw, formData.CanTriggerDraw)
	}
	if userData.CanViewLogs != formData.CanViewLogs {
		logsChange += fmt.Sprintf("can_view_logs: %t -> %t, ", userData.CanViewLogs, formData.CanViewLogs)
	}
	if userData.CanAddUser != formData.CanAddUser {
		logsChange += fmt.Sprintf("can_add_user: %t -> %t, ", userData.CanAddUser, formData.CanAddUser)
	}
	if logsChange == "" {
		return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "No changes made")
	}
	_, err = config.DB.Exec(ctx,
		`update users set fname=$1,lname=$2,email=$3,phone=$4,department_id=$5,can_add_codes=$6,can_trigger_draw=$7,can_view_logs=$8,can_add_user=$9 where id=$10`,
		formData.Fname, formData.Lname, formData.Email, formData.Phone, formData.Department, formData.CanAddCode, formData.CanTriggerDraw, formData.CanViewLogs, formData.CanAddUser, userId)

	if err != nil {
		if ok, key := utils.IsErrDuplicate(err); ok {
			return utils.JsonErrorResponse(c, fiber.StatusConflict, fmt.Sprintf("Unable to update user data, %s already exists", key))
		} else if ok, key := utils.IsForeignKeyErr(err); ok {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, fmt.Sprintf("Unable to update user data, %s is invalid", key))
		}
		responseStatus = fiber.StatusConflict
		c.SendStatus(responseStatus)
		return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Unable to update data, system error. please try again later", utils.Logger{
			LogLevel:    utils.CRITICAL,
			Message:     fmt.Sprintf("CreatePrizeType: Unable to update data, Name:%s, err:%v", formData.Fname, err),
			ServiceName: config.ServiceName,
		})
	}

	utils.RecordActivityLog(config.DB,
		utils.ActivityLog{
			UserID:       userPayload.Id,
			ActivityType: "updateUser",
			Description:  "update user data, " + logsChange,
			Status:       "success",
			IPAddress:    c.IP(),
			UserAgent:    c.Get("User-Agent"),
		},
		config.ServiceName,
		nil,
	)
	return c.JSON(fiber.Map{"status": responseStatus, "message": formData.Fname + " updated successfully"})
}
func PlayerMetrics(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type PrizeOverview struct {
		Province string `json:"province"`
		Mno      string `json:"mno"`
		Count    int    `json:"count"`
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	if startDateStr == "" {
		//set default to today
		startDateStr = time.Now().Format("2006-01-02")
	}
	if endDateStr == "" {
		//set default to today
		startDateStr = time.Now().Format("2006-01-02")
	}
	dateFilter := ""
	args := []interface{}{}
	var startDate time.Time
	if len(startDateStr) != 0 {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid start date provided")
		}
		dateFilter += "e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= $1"
		args = append(args, startDateStr)
	}
	if len(endDateStr) != 0 {
		endDate, err := time.Parse("2006-01-02", endDateStr)
		//check if end date is after start date
		if len(startDateStr) != 0 {
			if endDate.Before(startDate) {
				return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "End date should be after start date")
			}
		}
		//add one day to include the end date
		endDate = endDate.AddDate(0, 0, 1)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid end date provided")
		}
		argName := "$1"
		if len(dateFilter) != 0 {
			dateFilter += " and "
			argName = "$2"
		}
		args = append(args, endDate)
		dateFilter += "e.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= " + argName
	}
	if len(dateFilter) != 0 {
		dateFilter = " where " + dateFilter
	}
	query := fmt.Sprintf(`select count(e.id) as count,p.name as province,c.network_operator from entries e
	INNER JOIN customer c ON c.id=e.customer_id INNER JOIN province p ON p.id=c.province %s group by c.network_operator,p.id`, dateFilter)
	fmt.Println(query)
	rows, err := config.DB.Query(ctx, query, args...)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get player metrics data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "PlayerMetrics: Unable to get player metrics data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "player metrics data is not valid")
	}
	metrics := map[string]any{
		"east": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
		"north": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
		"south": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
		"west": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
		"kigali": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
	}
	for rows.Next() {
		prizeOverview := PrizeOverview{}
		err = rows.Scan(&prizeOverview.Count, &prizeOverview.Province, &prizeOverview.Mno)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get player metrics data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "PlayerMetrics: Unable to get player metrics data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		if prizeOverview.Province == "Eastern Province" {
			prizeOverview.Province = "east"
		} else if prizeOverview.Province == "Northern Province" {
			prizeOverview.Province = "north"
		} else if prizeOverview.Province == "Southern Province" {
			prizeOverview.Province = "south"
		} else if prizeOverview.Province == "Western Province" {
			prizeOverview.Province = "west"
		} else if prizeOverview.Province == "City of Kigali" {
			prizeOverview.Province = "kigali"
		}
		if metrics[prizeOverview.Province] == nil {
			metrics[prizeOverview.Province] = map[string]any{
				"mtn":    0,
				"airtel": 0,
			}
		}
		metrics[prizeOverview.Province].(map[string]any)[strings.ToLower(prizeOverview.Mno)] = prizeOverview.Count
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": metrics})
}
func WinnerMetrics(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type PrizeOverview struct {
		Province string `json:"province"`
		Mno      string `json:"mno"`
		Count    int    `json:"count"`
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	if startDateStr == "" {
		//set default to today
		startDateStr = time.Now().Format("2006-01-02")
	}
	if endDateStr == "" {
		//set default to today
		startDateStr = time.Now().Format("2006-01-02")
	}
	dateFilter := ""
	args := []interface{}{}
	var startDate time.Time
	if len(startDateStr) != 0 {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid start date provided")
		}
		dateFilter += "pr.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= $1"
		args = append(args, startDateStr)
	}
	if len(endDateStr) != 0 {
		endDate, err := time.Parse("2006-01-02", endDateStr)
		//check if end date is after start date
		if len(startDateStr) != 0 {
			if endDate.Before(startDate) {
				return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "End date should be after start date")
			}
		}
		//add one day to include the end date
		endDate = endDate.AddDate(0, 0, 1)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid end date provided")
		}
		argName := "$1"
		if len(dateFilter) != 0 {
			dateFilter += " and "
			argName = "$2"
		}
		args = append(args, endDate)
		dateFilter += "pr.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= " + argName
	}
	if len(dateFilter) != 0 {
		dateFilter = " where " + dateFilter
	}
	query := fmt.Sprintf(`select count(pr.id) as count,p.name as province,c.network_operator from prize pr
	INNER JOIN entries e ON pr.entry_id=e.id
	INNER JOIN customer c ON c.id=e.customer_id INNER JOIN province p ON p.id=c.province %s group by c.network_operator,p.id`, dateFilter)
	fmt.Println(query)
	rows, err := config.DB.Query(ctx, query, args...)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get winner metrics data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "WinnerMetrics: Unable to get winner metrics data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "winner metrics data is not valid")
	}
	metrics := map[string]any{
		"east": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
		"north": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
		"south": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
		"west": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
		"kigali": map[string]any{
			"mtn":    0,
			"airtel": 0,
		},
	}
	for rows.Next() {
		prizeOverview := PrizeOverview{}
		err = rows.Scan(&prizeOverview.Count, &prizeOverview.Province, &prizeOverview.Mno)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get player metrics data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "WinnerMetrics: Unable to get player metrics data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		if prizeOverview.Province == "Eastern Province" {
			prizeOverview.Province = "east"
		} else if prizeOverview.Province == "Northern Province" {
			prizeOverview.Province = "north"
		} else if prizeOverview.Province == "Southern Province" {
			prizeOverview.Province = "south"
		} else if prizeOverview.Province == "Western Province" {
			prizeOverview.Province = "west"
		} else if prizeOverview.Province == "City of Kigali" {
			prizeOverview.Province = "kigali"
		}
		if metrics[prizeOverview.Province] == nil {
			metrics[prizeOverview.Province] = map[string]any{
				"mtn":    0,
				"airtel": 0,
			}
		}
		metrics[prizeOverview.Province].(map[string]any)[strings.ToLower(prizeOverview.Mno)] = prizeOverview.Count
	}
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "data": metrics})
}

func GetPrizeOverviewV2(c *fiber.Ctx) error {
	_, err := utils.SecurePath(c, config.Redis)
	if err != nil {
		return utils.JsonErrorResponse(c, fiber.StatusUnauthorized, err.Error())
	}
	type PrizeOverview struct {
		TotalPrize      float64 `json:"total_prize"`
		PrizeCount      int     `json:"prize_count"`
		TotalEligibilty float64 `json:"total_elligibility"`
		PrizeType       string  `json:"prize_type"`
		Status          string  `json:"status"`
	}
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	if startDateStr == "" {
		//set default to today
		startDateStr = time.Now().Format("2006-01-02")
	}
	if endDateStr == "" {
		//set default to today
		startDateStr = time.Now().Format("2006-01-02")
	}
	dateFilter := ""
	args := []interface{}{}
	var startDate time.Time
	if len(startDateStr) != 0 {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid start date provided")
		}
		dateFilter += "p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' >= $1"
		args = append(args, startDateStr)
	}
	if len(endDateStr) != 0 {
		endDate, err := time.Parse("2006-01-02", endDateStr)
		//check if end date is after start date
		if len(startDateStr) != 0 {
			if endDate.Before(startDate) {
				return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "End date should be after start date")
			}
		}
		//add one day to include the end date
		endDate = endDate.AddDate(0, 0, 1)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusNotAcceptable, "Invalid end date provided")
		}
		argName := "$1"
		if len(dateFilter) != 0 {
			dateFilter += " and "
			argName = "$2"
		}
		args = append(args, endDate)
		dateFilter += "p.created_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Kigali' <= " + argName
	}
	if len(dateFilter) != 0 {
		dateFilter = " where " + dateFilter
	}
	prizeOverviews := []PrizeOverview{}
	query := fmt.Sprintf(`select sum(p.prize_value),count(p.id),pt.elligibility,t.status,pt.name from prize p
	INNER JOIN prize_type pt ON pt.id=p.prize_type_id INNER JOIN transaction t ON p.id=t.prize_id %s group by pt.id,t.status`, dateFilter)
	rows, err := config.DB.Query(ctx, query, args...)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prizeOverview data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeOverviewV2: Unable to get prizeOverview data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		return utils.JsonErrorResponse(c, fiber.StatusForbidden, "prizeOverview data is not valid")
	}
	for rows.Next() {
		prizeOverview := PrizeOverview{}
		err = rows.Scan(&prizeOverview.TotalPrize, &prizeOverview.PrizeCount, &prizeOverview.TotalEligibilty, &prizeOverview.Status, &prizeOverview.PrizeType)
		if err != nil {
			return utils.JsonErrorResponse(c, fiber.StatusInternalServerError, "Get prizeOverview data failed", utils.Logger{
				LogLevel:    utils.CRITICAL,
				Message:     "GetPrizeOverviewV2: Unable to get prizeOverview data, error: " + err.Error(),
				ServiceName: config.ServiceName,
			})
		}
		if prizeOverview.Status != "SUCCESS" {
			prizeOverview.Status = "PENDING"
		}
		prizeOverviews = append(prizeOverviews, prizeOverview)
	}
	data := map[string]any{
		"payouts": map[string]any{
			"expected": 0,
			"actual":   0,
		},
		"winners": map[string]any{
			"paid":    0,
			"pending": 0,
		},
		"rewards": map[string]any{
			"distributed": 0,
			"total":       0,
		},
	}
	loadedType := make(map[string]bool)
	var expected, distributed, total float64
	for _, prizeOverview := range prizeOverviews {
		expected += prizeOverview.TotalPrize
		if ok, _ := loadedType[prizeOverview.PrizeType]; !ok {
			loadedType[prizeOverview.PrizeType] = true
			total += prizeOverview.TotalEligibilty
		}
		distributed += float64(prizeOverview.PrizeCount)
		if prizeOverview.Status == "SUCCESS" {
			data["payouts"].(map[string]any)["actual"] = data["payouts"].(map[string]any)["actual"].(int) + int(prizeOverview.TotalPrize)
			data["winners"].(map[string]any)["paid"] = data["winners"].(map[string]any)["paid"].(int) + prizeOverview.PrizeCount
		} else {
			data["winners"].(map[string]any)["pending"] = data["winners"].(map[string]any)["pending"].(int) + prizeOverview.PrizeCount
		}
		if prizeOverview.PrizeType == "REWARD" {
			data["rewards"].(map[string]any)["distributed"] = data["rewards"].(map[string]any)["distributed"].(int) + prizeOverview.PrizeCount
			data["rewards"].(map[string]any)["total"] = data["rewards"].(map[string]any)["total"].(int) + int(prizeOverview.TotalEligibilty)
		}
	}
	data["payouts"].(map[string]any)["expected"] = expected
	data["rewards"].(map[string]any)["distributed"] = distributed
	data["rewards"].(map[string]any)["total"] = total
	return c.JSON(fiber.Map{"status": fiber.StatusOK, "message": "success", "prize_overview": data})
}
