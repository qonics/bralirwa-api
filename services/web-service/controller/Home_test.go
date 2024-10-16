package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"shared-package/utils"
	"testing"

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
				"password": "password123",
			},
			expectedCode: 200,
			expectedBody: "Login completed",
			expectedData: map[string]interface{}{
				"status":  200,
				"message": "Login completed",
				"user_data": map[string]interface{}{
					"Username": "test",
					"Email":    "test@qonics.com",
					"Names":    "Test user",
					"Status":   float64(1), // JSON unmarshalling converts numbers to floats
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

		// Check the response body contains expected text
		a.Contains(string(body), test.expectedBody, test.description)

		if test.expectedData != nil {
			var result map[string]interface{}
			json.Unmarshal(body, &result)

			// Check detailed fields for successful login
			if test.description == "successful login" {
				userData, ok := result["user_data"].(map[string]interface{})
				a.True(ok, "user_data should be a map")

				// Check each field
				a.Equal(test.expectedData["user_data"].(map[string]interface{})["Username"], userData["Username"])
				a.Equal(test.expectedData["user_data"].(map[string]interface{})["Email"], userData["Email"])
				a.Equal(test.expectedData["user_data"].(map[string]interface{})["Names"], userData["Names"])
				a.Equal(test.expectedData["user_data"].(map[string]interface{})["Status"], userData["Status"])
			}
		}
	}
}

// Mock configurations
func init() {
	utils.IsTestMode = true
	viper.Set("saltKey", "testSaltKey")
}
