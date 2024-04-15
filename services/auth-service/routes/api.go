package routes

import (
	"auth-service/controller"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/utils"
)

func InitRoutes() *fiber.App {

	// v1 := r.Group("/api/v1/")

	// v1.GET("service-status", controller.ServiceStatusCheck)
	// v1.GET("/", controller.Index)

	app := fiber.New(fiber.Config{
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,
	})
	app.Use(recover.New())
	// app.Use(logger.New())
	app.Use(cors.New())
	// app.Use(cors.New(cors.Config{
	// 	AllowOrigins: "*",
	// 	AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	// 	AllowMethods: "GET, HEAD, PUT, PATCH, POST, DELETE",
	// }))
	// app.Use(limiter.New(limiter.Config{
	// 	Max:        20,
	// 	Expiration: time.Second * 60,
	// }))

	//CSRF protection
	app.Use(csrf.New(csrf.Config{
		KeyLookup:      "header:X-Csrf-Token",
		CookieName:     "csrf_",
		CookieSameSite: "Lax",
		Expiration:     1 * time.Hour,
		KeyGenerator:   utils.UUIDv4,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			accepts := c.Accepts("html", "json")
			path := c.Path()
			if accepts == "json" || strings.HasPrefix(path, "/api/") {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"message": "Forbidden",
				})
			}
			return c.Status(fiber.StatusForbidden).Render("error", fiber.Map{
				"Title":  "Forbidden",
				"Status": fiber.StatusForbidden,
			}, "templates/forbidden")
		},
	}))
	v1 := app.Group("/auth/api/v1/")
	v1.Get("/service-status", controller.ServiceStatusCheck)
	v1.Get("/", controller.Index)
	v1.Get("/test-logger", controller.TestLoggingService)
	v1.Post("/login", controller.LoginWithEmail)

	return app
}
