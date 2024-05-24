package controller

import (
	"auth-service/config"
	"auth-service/model"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"shared-package/utils"
	"time"

	"libs/shared-package/proto"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
)

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

func TestLoggingService(c *fiber.Ctx) error {
	conn, err := grpc.Dial("logger-service:50051", grpc.WithInsecure())
	if err != nil {
		return c.JSON(fiber.Map{"status": 500, "message": "Logger service not connected: " + err.Error()})
	}
	defer conn.Close()
	client := proto.NewLoggerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := client.Log(ctx, &proto.LogRequest{LogLevel: "info", LogTime: time.Now().Format(time.DateTime),
		ServiceName: "auth-service", Message: "Hello log test", Identifier: utils.RandString(12)})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
		return c.JSON(fiber.Map{"status": 500, "message": "Logger service not responsed: " + err.Error()})
	}
	log.Printf("Response: %s", r.GetResponse())
	return c.JSON(fiber.Map{"status": 200, "message": "Logger service response: " + r.GetResponse()})
}

func LoginWithEmail(c *fiber.Ctx) error {
	type UserData struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	responseStatus := 200
	userData := new(UserData)
	if err := c.BodyParser(userData); err != nil || userData.Email == "" {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data", "details": err})
	}

	//TODO: login logic goes here
	if userData.Email != "test@qonics.com" {
		responseStatus = 403
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Invalid credentials"})
	}
	//TODO: Load user data
	data := model.UserProfile{Username: "test", Email: userData.Email, Names: "Test user", Status: 1}
	c.SendStatus(responseStatus)
	return c.JSON(fiber.Map{"status": responseStatus, "message": "Login completed", "user_data": data})
}

func LoginWithGoogle(c *fiber.Ctx) error {
	localState := viper.GetString("saltKey")
	url := config.AppConfig.GoogleLoginConfig.AuthCodeURL(localState)
	c.Redirect(url)
	return c.JSON(url)
}

func GoogleCallback(c *fiber.Ctx) error {
	// fmt.Println("Google oauth callback received, ", c.Queries())
	state := c.Query("state")
	localState := viper.GetString("saltKey")
	if state != localState {
		fmt.Println("LoginWithGoogle, State received: ", state, " | compare:", localState)
		return c.SendString("States doesn't match")
	}

	code := c.Query("code")
	googleCon := config.GoogleConfig()

	token, err := googleCon.Exchange(context.Background(), code)
	if err != nil {
		return c.SendString("Code-Token Exchange Failed:" + err.Error())
	}
	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		return c.SendString(fmt.Sprintf("User Data Fetch Failed, %v", err.Error()))
	}
	if resp.Body == nil {
		return c.SendString("Response body is nil")
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.SendString(fmt.Sprintf("User body decoding failed, %v", err.Error()))
	}
	// body, _ := ioutil.ReadAll(response.Body)
	userData := string(body)
	//TODO: use this data to create or authenticate user
	return c.SendString(string(userData))
}

func LoginWithGithub(c *fiber.Ctx) error {
	localState := viper.GetString("saltKey")
	url := config.AppConfig.GithubLoginConfig.AuthCodeURL(localState, oauth2.AccessTypeOffline)
	// url := fmt.Sprintf("https://github.com/login/oauth/authorize%s", urlParams)
	c.Redirect(url)
	return c.JSON(url)
}

func GithubCallback(c *fiber.Ctx) error {
	state := c.Query("state")
	localState := viper.GetString("saltKey")
	if state != localState {
		fmt.Println("LoginWithGithub, State received: ", state, " | compare:", localState)
		return c.SendString("States doesn't match")
	}

	code := c.Query("code")
	githubCon := config.GithubConfig()

	token, err := githubCon.Exchange(context.Background(), code)
	if err != nil {
		return c.SendString("Code-Token Exchange Failed, " + err.Error())
	}
	request, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return c.SendString(fmt.Sprintf("Github User Data Fetch Failed, %v", err.Error()))
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return c.SendString(fmt.Sprintf("Github User Data Fetch Failed, %v", err.Error()))
	}
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.SendString(fmt.Sprintf("User body decoding failed, %v", err.Error()))
	}
	userData := string(body)
	//TODO: use this data to create or authenticate user
	return c.SendString(string(userData))
}
