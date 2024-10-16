package routes

import (
	"fmt"
	"shared-package/utils"
	"strings"
	"time"
	"ussd-service/controller"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/recover"
	html "github.com/gofiber/template/html/v2"
)

func InitRoutes() *fiber.App {
	engine := html.New("/app/templates", ".html")
	app := fiber.New(fiber.Config{
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,
		Views:       engine,
	})
	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(csrf.New(csrf.Config{
		KeyLookup:      "header:X-Csrf-Token",
		CookieName:     "csrf_",
		CookieSameSite: "Strict",
		Expiration:     1 * time.Hour,
		KeyGenerator:   utils.GenerateCSRFToken,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			fmt.Println("CSRF error")
			accepts := c.Accepts("html", "json")
			path := c.Path()
			if accepts == "json" || strings.HasPrefix(path, "/auth/api/") {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"status":  fiber.StatusForbidden,
					"message": "Forbidden: You are not allowed to access this resource",
				})
			}

			return c.Status(fiber.StatusForbidden).Render("forbidden", fiber.Map{
				"Title":  "Forbidden",
				"Status": fiber.StatusForbidden,
				"Path":   path,
			})
		},
	}))
	v1 := app.Group("/ussd/api/v1/")
	v1.All("/service-status", controller.ServiceStatusCheck)
	v1.Get("/webhook", controller.USSDService)

	return app
}
