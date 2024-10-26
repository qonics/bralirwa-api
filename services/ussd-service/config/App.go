package config

import (
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
)

var Redis *redis.Client
var EncryptionKey string
var ServiceName string = "web-service"

func InitializeConfig() {
	EncryptionKey = viper.GetString("encryption_key")
	Redis = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", viper.GetString("redis.host"), viper.GetString("redis.port")),
		Password: viper.GetString("redis.password"),
		DB:       viper.GetInt("redis.database"),
	})
}
