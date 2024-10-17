package main

import (
	"fmt"
	"shared-package/utils"
	"web-service/config"
	"web-service/routes"
)

func main() {
	fmt.Println("Hello - web-service: 9000")
	utils.InitializeViper("config", "yml")
	config.InitializeConfig()
	config.ConnectDb()
	defer config.DB.Close()
	server := routes.InitRoutes()
	server.Listen("0.0.0.0:9000")
}
