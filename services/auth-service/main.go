package main

import (
	"auth-service/config"
	"auth-service/routes"
	"fmt"
)

func main() {
	fmt.Println("Hello - my-service: 9000")
	config.InitializeConfig()
	config.ConnectDb()
	defer config.SESSION.Close()
	defer config.DB.Close()
	config.GoogleConfig()
	server := routes.InitRoutes()
	server.Listen("0.0.0.0:9000")
}
