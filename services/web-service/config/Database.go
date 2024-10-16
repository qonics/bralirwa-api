package config

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq" // PostgreSQL driver for database/sql
	"github.com/spf13/viper"
)

var SESSION *pgxpool.Pool

func ConnectDb() {
	// Read configuration from environment variables
	fmt.Println("Connecting to database...", viper.GetString("postgres_db.cluster"))
	user := viper.GetString("postgres_db.user")
	password := viper.GetString("postgres_db.password")
	host := viper.GetString("postgres_db.cluster")
	port := viper.GetInt("postgres_db.port")
	dbname := viper.GetString("postgres_db.keyspace")

	// Construct the connection string
	databaseUrl := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, password, host, port, dbname)

	// Step 1: Create a *sql.DB connection for migrations
	migrationDb, err := sql.Open("postgres", databaseUrl)
	if err != nil {
		log.Fatalf("Error opening database connection for migrations: %v", err)
	}

	// Step 2: Run automatic migrations
	runMigrations(migrationDb)

	// Step 3: Create pgxpool.Pool connection for your application
	dbConfig, err := pgxpool.ParseConfig(databaseUrl)
	if err != nil {
		log.Fatalf("Failed to create pgxpool config: %v", err)
	}
	dbConfig.MaxConns = 4
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

	// Assign to global variable
	SESSION = pool

	log.Println("Database connected and ready for application use!")
}

func runMigrations(db *sql.DB) {
	// Step 4: Run the migrations using golang-migrate with *sql.DB
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("Could not create postgres driver: %v", err)
	}

	// Define the path to your migrations folder
	migrationDir := "file://app/migration"

	// Create a new migrator instance
	migrator, err := migrate.NewWithDatabaseInstance(migrationDir, "postgres", driver)
	if err != nil {
		log.Fatalf("Failed to initialize migrations: %v", err)
	}

	// Apply the migrations
	if err := migrator.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Migrations applied successfully!")
}
