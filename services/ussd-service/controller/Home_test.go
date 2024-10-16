package controller

import (
	"shared-package/utils"

	"github.com/spf13/viper"
)

// Mock configurations
func init() {
	utils.IsTestMode = true
	viper.Set("saltKey", "testSaltKey")
}
