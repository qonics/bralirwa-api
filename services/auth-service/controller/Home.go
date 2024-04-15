package controller

import (
	"auth-service/helper"
	"auth-service/model"
	"context"
	"log"
	"time"

	"libs/shared-package/proto"

	"github.com/gofiber/fiber/v2"
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
	return c.JSON(fiber.Map{"status": 200, "message": "This API service is running"})
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
		ServiceName: "auth-service", Message: "Hello log test", Identifier: helper.RandString(12)})
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
		Password string `json:"string" binding:"required"`
	}
	responseStatus := 200
	userData := new(UserData)
	if err := c.BodyParser(userData); err != nil {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data"})
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
	type UserData struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"string" binding:"required"`
	}
	responseStatus := 200
	userData := new(UserData)
	if err := c.BodyParser(userData); err != nil {
		responseStatus = 400
		c.SendStatus(responseStatus)
		return c.JSON(fiber.Map{"status": responseStatus, "message": "Please provide all required data"})
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
