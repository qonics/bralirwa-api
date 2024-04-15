package config

import (
	"fmt"
	"os"
)

func InitializeConfig() {
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal("Error loading .env file, system will load default value")
	// }
	fmt.Println("App mode: ", os.Getenv("APP_MODE"))
	// if os.Getenv("APP_MODE") == "release" {
	// 	gin.SetMode(gin.ReleaseMode)
	// } else {
	// 	gin.SetMode(gin.DebugMode)
	// }
}
