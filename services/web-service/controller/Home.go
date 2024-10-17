package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"logger-service/helper"
	"strings"
	"time"
	"web-service/config"
	"web-service/model"

	"shared-package/utils"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

var Validate = validator.New()
var ctx = context.Background()

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
	u.can_add_codes,u.can_trigger_draw,u.can_add_user,u.can_view_logs from users u inner join departments d on u.department_id = d.id where email = $1 and password = crypt($2, password)`, userData.Email, userData.Password).
		Scan(&UserProfile.Id, &UserProfile.Fname, &UserProfile.Lname, &UserProfile.Department.Id, &UserProfile.Department.Title, &UserProfile.EmailVerified, &UserProfile.PhoneVerified, &UserProfile.AvatarUrl, &UserProfile.Status,
			&UserProfile.CanAddCodes, &UserProfile.CanTriggerDraw, &UserProfile.CanAddUser, &UserProfile.CanViewLogs)
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
	fmt.Println("debug 4")
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
