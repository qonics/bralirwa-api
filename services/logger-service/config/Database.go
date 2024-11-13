package config

import (
	"context"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
)

var DB *pgxpool.Pool

func ConnectDb() {
	// Read configuration from environment variables
	user := viper.GetString("postgres_db.user")
	password := viper.GetString("postgres_db.password")
	host := viper.GetString("postgres_db.cluster")
	port := viper.GetInt("postgres_db.port")
	dbname := viper.GetString("postgres_db.keyspace")
	// Construct the connection string
	databaseUrl := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, password, host, port, dbname)
	// Step 3: Create pgxpool.Pool connection for your application
	dbConfig, err := pgxpool.ParseConfig(databaseUrl)
	if err != nil {
		log.Fatalf("Failed to create pgxpool config: %v", err)
	}
	dbConfig.MaxConns = 20
	dbConfig.MinConns = 0
	dbConfig.MaxConnLifetime = time.Hour
	dbConfig.MaxConnIdleTime = 30 * time.Minute
	dbConfig.HealthCheckPeriod = time.Minute
	dbConfig.ConnConfig.ConnectTimeout = 5 * time.Second
	// Create pgxpool.Pool for application use
	pool, err := pgxpool.NewWithConfig(context.Background(), dbConfig)
	if err != nil {
		log.Fatalf("Error while creating pgxpool connection: %v", err)
	}
	DB = pool
	//Gorm ORM: Mysql, postgress,..
	// connectionUrl := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8&parseTime=True&loc=Local", os.Getenv("user"), os.Getenv("password"), os.Getenv("host"), os.Getenv("port"), os.Getenv("database"))
	// fmt.Println(connectionUrl)
	// database, err := gorm.Open("mysql", connectionUrl)
	// fmt.Println(connectionUrl)
	// if err != nil {
	// 	panic("Database connection error " + err.Error())
	// }
	// database.AutoMigrate(&model.AssessmentDistribution{})
}
