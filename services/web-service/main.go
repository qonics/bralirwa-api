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
	//initialize airtel smpp connection
	// go func() {
	// 	config.AirtelTX = config.InitializeSMPP(viper.GetString("smpp.airtel.address"), viper.GetString("smpp.airtel.user"), viper.GetString("smpp.airtel.password"), false)
	// }()
	// go func() {
	// 	config.MTNTx = config.InitializeSMPP(viper.GetString("smpp.mtn.address"), viper.GetString("smpp.mtn.user"), viper.GetString("smpp.mtn.password"), false)
	// }()
	defer config.DB.Close()
	server := routes.InitRoutes()
	server.Listen("0.0.0.0:9000")
}
