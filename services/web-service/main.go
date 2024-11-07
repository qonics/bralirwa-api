package main

import (
	"fmt"
	"shared-package/utils"
	"web-service/config"
	"web-service/controller"
	"web-service/routes"
)

func main() {
	fmt.Println("Hello - web-service: 9000")
	utils.InitializeViper("config", "yml")
	config.InitializeConfig()
	config.ConnectDb()
	go controller.DistributeMomoPrize()
	defer config.DB.Close()
	server := routes.InitRoutes()
	server.Listen("0.0.0.0:9000")
}
