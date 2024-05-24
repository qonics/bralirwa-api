package config

import (
	"fmt"
	"os"
	"shared-package/utils"

	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

func InitializeConfig() {
	// err := godotenv.Load()
	// if err != nil {
	// 	log.Fatal("Error loading .env file, system will load default value")
	// }
	fmt.Println("App mode: ", os.Getenv("APP_MODE"))

}

type Config struct {
	GoogleLoginConfig oauth2.Config
	GithubLoginConfig oauth2.Config
}

var AppConfig Config

func GoogleConfig() oauth2.Config {
	utils.InitializeViper()

	AppConfig.GoogleLoginConfig = oauth2.Config{
		RedirectURL:  "http://127.0.0.1:9080/auth/api/v1/google/callback",
		ClientID:     viper.GetString("google.clientID"),
		ClientSecret: viper.GetString("google.clientSecret"),
		Scopes: []string{"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint: google.Endpoint,
	}
	return AppConfig.GoogleLoginConfig
}

func GithubConfig() oauth2.Config {
	utils.InitializeViper()

	AppConfig.GithubLoginConfig = oauth2.Config{
		RedirectURL:  "http://127.0.0.1:9080/auth/api/v1/github/callback",
		ClientID:     viper.GetString("github.clientID"),
		ClientSecret: viper.GetString("github.clientSecret"),
		Scopes:       []string{"user"},
		Endpoint:     github.Endpoint,
	}
	return AppConfig.GithubLoginConfig
}
