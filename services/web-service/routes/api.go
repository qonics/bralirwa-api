package routes

import (
	"fmt"
	"shared-package/utils"
	"strings"
	"time"
	"web-service/controller"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/recover"
	html "github.com/gofiber/template/html/v2"
)

func InitRoutes() *fiber.App {

	// v1 := r.Group("/api/v1/")

	// v1.GET("service-status", controller.ServiceStatusCheck)
	// v1.GET("/", controller.Index)
	engine := html.New("/app/templates", ".html")
	app := fiber.New(fiber.Config{
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,
		Views:       engine,
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
	v1 := app.Group("/auth/api/v1/")
	v1.All("/service-status", func(c *fiber.Ctx) error {
		fmt.Println("Calling home endpoint")
		return c.JSON(fiber.Map{"status": 200, "message": "This API service is running!"})
	})
	v1.Get("/csrf-token", func(c *fiber.Ctx) error {
		//trigger new token saving in cookie before fetching to ensure we got a collect CSRF Token from csrf middleware
		c.ClearCookie("csrf_")
		// _, err := http.Get("http://127.0.0.1:9000/auth/api/v1/service-status")

		// if err != nil {
		// 	return c.JSON(fiber.Map{"status": fiber.StatusInternalServerError, "message": "Fetching CSRF Token failed, please try again"})
		// }
		csrfToken := c.Cookies("csrf_", "NONE")
		fmt.Println("CSRF Token: ", csrfToken)
		// if csrfToken == "NONE" {
		// 	csrfToken = helper.GenerateCSRFToken()

		// 	fmt.Println("Generated CSRF Token: ", csrfToken)
		// 	c.Cookie(&fiber.Cookie{
		// 		Name:     "csrf_",
		// 		Value:    csrfToken,
		// 		SameSite: "Strict",
		// 		HTTPOnly: true,
		// 	})
		// }
		return c.JSON(fiber.Map{"csrfToken": csrfToken})
	})
	v1.Get("/", controller.Index)
	v1.Post("/login", controller.LoginWithEmail)

	return app
}
