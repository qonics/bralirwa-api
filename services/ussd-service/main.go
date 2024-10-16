package main

import (
	"fmt"
	"shared-package/utils"
	"ussd-service/config"
	"ussd-service/routes"

	"github.com/spf13/viper"
)

func main() {
	fmt.Println("Hello - ussd-service: 9000")
	utils.InitializeViper("config", "yml")
	config.InitializeConfig()
	//load ussd config
	utils.InitializeViper("ussd_config", "json")

	// Convert steps into Viper keys with id as the key
	steps := viper.Get("steps").([]interface{})
	for _, step := range steps {
		step := step.(map[string]interface{}) // Cast to map
		viper.Set(fmt.Sprintf("steps.%s", step["id"]), step)
	}
	// fmt.Println(viper.Get("steps"))
	// fmt.Println(viper.Get("steps.action_ack.inputs"))
	config.ConnectDb()
	defer config.DB.Close()
	server := routes.InitRoutes()
	server.Listen("0.0.0.0:9000")
}
