package controller

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func TestLoginWithEmail(t *testing.T) {
	app := fiber.New()
	c := app.AcquireCtx(&fasthttp.RequestCtx{})
	data := struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}{
		Email:    "test@qonics.com",
		Password: "1231231",
	}
	req, err := json.Marshal(data)
	if err != nil {
		t.Error("Error occurred while encoding json request", err)
	}
	fmt.Println(string(req))
	c.Request().SetBody(req)

	err = LoginWithEmail(c)
	if err != nil {
		t.Error("Error occurred while decoding json response", err)
	}
	// Convert the JSON response to a map
	var response map[string]any
	err = json.Unmarshal([]byte(c.Response().Body()), &response)
	if err != nil {
		t.Error("Error occurred while decoding json response", err)
	}
	if response["status"] != "200" {
		t.Error("Failed: ", response)
	}
}
