package controller

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func TestLoginWithEmail(t *testing.T) {
	app := fiber.New()
	c := app.AcquireCtx(&fasthttp.RequestCtx{})
	result := LoginWithEmail(c)

	if result.Error() != "sdasd" {
		t.Error("Invalid response")
	}
}
