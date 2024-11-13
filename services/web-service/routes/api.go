package routes

import (
	"fmt"
	"time"
	"web-service/controller"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	html "github.com/gofiber/template/html/v2"
)

func InitRoutes() *fiber.App {

	// v1 := r.Group("/api/v1/")

	// v1.GET("service-status", controller.ServiceStatusCheck)
	// v1.GET("/", controller.Index)
	engine := html.New("/app/templates", ".html")
	app := fiber.New(fiber.Config{
		JSONEncoder:  json.Marshal,
		JSONDecoder:  json.Unmarshal,
		Views:        engine,
		ReadTimeout:  time.Minute * 20,  // Increase read timeout (e.g., 5 minutes)
		WriteTimeout: time.Minute * 20,  // Increase write timeout (e.g., 5 minutes)
		BodyLimit:    100 * 1024 * 1024, // 50 MB limit
	})
	app.Use(recover.New())
	// app.Use(logger.New())
	app.Use(cors.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:3000",
		AllowHeaders:     "Content-Type, Access-Control-Allow-Headers, Authorization, X-Requested-With, x-csrf-token",
		AllowMethods:     "*",
		AllowCredentials: false,
	}))

	v1 := app.Group("/api/v1/")
	v1.All("/service-status", func(c *fiber.Ctx) error {
		fmt.Println("Calling home endpoint")
		return c.JSON(fiber.Map{"status": 200, "message": "This API service is running!"})
	})
	v1.Get("/", controller.Index)
	v1.Post("/login", controller.LoginWithEmail)
	v1.Get("/profile", controller.GetUserProfile)

	v1.Get("/prize_categories", controller.GetPrizeCategory)
	v1.Get("/prize_type/:prize_category?", controller.GetPrizeType)
	v1.Get("/prizes", controller.GetPrizeType)
	v1.Post("/prize_category", controller.CreatePrizeCategory)
	v1.Post("/prize_type", controller.CreatePrizeType)
	v1.Post("/user", controller.AddUser)
	v1.Get("/users", controller.GetUsers)
	v1.Get("/entries", controller.GetEntries)
	v1.Get("/draws", controller.GetDraws)
	v1.Get("/prize_distributions", controller.GetPrizes)
	v1.Get("/customer/:customerId", controller.GetCustomer)
	v1.Get("/entry/:entryId", controller.GetEntryData)
	v1.Get("/customer_entry_history/:customerId", controller.GetUserProfile)
	v1.Post("/upload_codes", controller.UploadCodes)
	v1.Post("/change_password", controller.ChangePassword)
	v1.Post("/forgot_password", controller.ForgotPassword)
	v1.Post("/set_password", controller.SetNewPassword)
	v1.Post("/validate_otp", controller.ValidateOTP)
	v1.Post("/verify_otp", controller.ValidateOTP)
	v1.Get("/avatar/svg/:type/:avatar_number", controller.GetSVGAvatar)
	v1.Get("/draws", controller.GetDraws)
	v1.Post("/draw", controller.StartPrizeDraw)
	v1.Get("/distribution-type", controller.GetDistributionType)
	v1.Get("/departments", controller.GetDepartments)
	v1.Get("/sms_sent", controller.GetSMSSent)
	v1.Get("/prize_overview", controller.GetPrizeOverview)
	v1.Get("/code-overview", controller.GetCodeOverview)
	v1.Get("/logs", controller.GetLogs)
	v1.Get("/sms_balance", controller.GetSMSBalance)
	v1.Post("/user_status/:userId", controller.ChangeUserStatus)
	v1.Get("/provinces", controller.GetProvinces)
	v1.Get("/transactions", controller.GetTransactions)
	v1.Get("/prize_type_space/:type_id", controller.GetPrizeTypeSpace)
	v1.Post("/confirm-trx/:transaction_id", controller.ConfirmTransaction)
	v1.Post("/confirm-bulk-trx/", controller.ConfirmBulkTransaction)
	return app
}
