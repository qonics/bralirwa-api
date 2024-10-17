package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"shared-package/utils"
	"testing"
	"web-service/config"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestLoginWithEmail(t *testing.T) {
	// Setup Fiber app
	app := fiber.New()

	// Define the route
	app.Post("/login", LoginWithEmail)

	// Test cases
	tests := []struct {
		description  string
		payload      map[string]string
		expectedCode int
		expectedBody string
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description: "successful login",
			payload: map[string]string{
				"email":    "test@qonics.com",
				"password": "password",
			},
			expectedCode: 200,
			expectedBody: "Login completed",
			expectedData: map[string]interface{}{
				"status":  200,
				"message": "Login completed",
				"data": map[string]interface{}{
					"can_add_codes": true,
					"email":         "test@qonics.com",
					"firstname":     "Admin",
					"lastname":      "User test",
					"status":        "OKAY", // JSON unmarshalling converts numbers to floats
				},
			},
		},
		{
			description: "invalid email",
			payload: map[string]string{
				"email":    "wrong@qonics.com",
				"password": "password123",
			},
			expectedCode: 403,
			expectedBody: "Invalid credentials",
		},
		{
			description:  "missing fields",
			payload:      map[string]string{},
			expectedCode: 400,
			expectedBody: "Please provide all required data",
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/login", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)

		// Read the response body
		body, _ := io.ReadAll(resp.Body)
		// fmt.Println(string(body))
		// Check the response body contains expected text
		a.Contains(string(body), test.expectedBody, test.description)

		if test.expectedData != nil {
			var result map[string]interface{}
			json.Unmarshal(body, &result)

			// Check detailed fields for successful login
			if test.expectedCode == 200 {
				userData, ok := result["data"].(map[string]interface{})
				a.True(ok, "data should be a map")

				// Check each field
				a.Equal(test.expectedData["data"].(map[string]interface{})["can_add_codes"], userData["can_add_codes"])
				a.Equal(test.expectedData["data"].(map[string]interface{})["email"], userData["email"])
				a.Equal(test.expectedData["data"].(map[string]interface{})["firstname"], userData["firstname"])
				a.Equal(test.expectedData["data"].(map[string]interface{})["status"], userData["status"])
			}
		}
	}
}

// Mock configurations
func init() {
	utils.IsTestMode = true
	viper.Set("saltKey", "testSaltKey")
	utils.IsTestMode = true
	utils.InitializeViper("config", "yml")
	viper.Set("saltKey", "testSaltKey")
	//set the test db config
	viper.Set("postgres_db.cluster", "127.0.0.1")
	viper.Set("postgres_db.keyspace", "lottery_db")
	viper.Set("postgres_db.password", viper.GetString("postgres_db_test.password"))
	config.ConnectDb()
	config.Redis = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", viper.GetString("redis_test.host"), viper.GetString("redis_test.port")),
		Password: viper.GetString("redis_test.password"),
		DB:       viper.GetInt("redis_test.database"),
	})
	//Create dummy users for testing
	config.DB.Exec(ctx, `INSERT INTO users (fname, lname, phone, email, can_add_codes,can_trigger_draw,can_add_user,can_view_logs, department_id, email_verified, phone_verified, locale, avatar_url, password, status, address, operator)
VALUES
('Admin', 'User test', 'NOT_AVAILABLE', 'test@qonics.com', true, true, true, true, 1, FALSE, FALSE, 'en', 'NOT_AVAILABLE',
 '$2a$06$GeEpPxbKoTn3tAkyufWilumzne1MvF4uw0Vl7/X/VsZ4DM.r3zWRi', 'OKAY', 'NOT_AVAILABLE', NULL);`)
	config.DB.Exec(ctx, `INSERT INTO users (fname, lname, phone, email, can_add_codes,can_trigger_draw,can_add_user,can_view_logs, department_id, email_verified, phone_verified, locale, avatar_url, password, status, address, operator)
VALUES
('Admin', 'User test 2', 'NOT_AVAILABLE', 'test2@qonics.com', false, false, false, true, 1, FALSE, FALSE, 'en', 'NOT_AVAILABLE',
 '$2a$06$GeEpPxbKoTn3tAkyufWilumzne1MvF4uw0Vl7/X/VsZ4DM.r3zWRi', 'OKAY', 'NOT_AVAILABLE', NULL);`)
}
