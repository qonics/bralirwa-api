package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"shared-package/utils"
	"testing"
	"time"
	"web-service/config"
	"web-service/model"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

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
	_, err := config.DB.Exec(ctx, `INSERT INTO users (fname, lname, phone, email, can_add_codes,can_trigger_draw,can_add_user,can_view_logs, department_id, email_verified, phone_verified, locale, avatar_url, password, status, address, operator)
VALUES
('Admin', 'User test', '078234234232', 'test@qonics.com', true, true, true, true, 1, FALSE, FALSE, 'en', 'NOT_AVAILABLE',
 '$2a$06$GeEpPxbKoTn3tAkyufWilumzne1MvF4uw0Vl7/X/VsZ4DM.r3zWRi', 'OKAY', 'NOT_AVAILABLE', NULL);`)
	if err != nil {
		fmt.Println("Error inserting user data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO users (fname, lname, phone, email, can_add_codes,can_trigger_draw,can_add_user,can_view_logs, department_id, email_verified, phone_verified, locale, avatar_url, password, status, address, operator)
VALUES
('Admin', 'User test 2', '078234234231', 'test2@qonics.com', false, false, false, true, 1, FALSE, FALSE, 'en', 'NOT_AVAILABLE',
 '$2a$06$GeEpPxbKoTn3tAkyufWilumzne1MvF4uw0Vl7/X/VsZ4DM.r3zWRi', 'OKAY', 'NOT_AVAILABLE', NULL);`)
	if err != nil {
		fmt.Println("Error inserting user data", err)
	}
	// save prize category
	_, err = config.DB.Exec(ctx, `INSERT INTO prize_category (id,name, status) VALUES (1,'Test Category 1', 'OKAY');`)
	if err != nil {
		fmt.Println("Error inserting prize_category data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO prize_category (id,name, status) VALUES (2,'Test Category 2', 'OKAY');`)
	if err != nil {
		fmt.Println("Error inserting prize_category data", err)
	}
	// save prize type
	_, err = config.DB.Exec(ctx, `INSERT INTO prize_type (id,name, prize_category_id, value, elligibility, status) VALUES (1,'Test Prize 1', 1, 100, 1, 'OKAY');`)
	if err != nil {
		fmt.Println("Error inserting prize_type data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO prize_type (id,name, prize_category_id, value, elligibility, status) VALUES (2,'Test Prize 2', 1, 200, 5, 'OKAY');`)
	if err != nil {
		fmt.Println("Error inserting prize_type data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO prize_type (id,name, prize_category_id, value, elligibility, status) VALUES (3,'Test Prize 3', 2, 100, 2, 'OKAY');`)
	if err != nil {
		fmt.Println("Error inserting prize_type data", err)
	}

}

func createTestAccessToken() string {
	userData := model.UserProfile{
		Id:             1,
		Fname:          "Test",
		Lname:          "user",
		Email:          "test@qonics.com",
		CanAddCodes:    true,
		CanTriggerDraw: true,
		CanAddUser:     true,
		CanViewLogs:    true,
		Status:         "OKAY",
	}
	payloadData, err := json.Marshal(userData)
	if err != nil {
		panic("Unable to marshal struct into json")
	}
	token := "dG9rZW5fYzM5MThmNjItMWM0Ny0xMWVmLWE5NjUtMDI0MjBhMTQwMDA2XzE3MTY5NjU4NTY5NjS"
	if err := config.Redis.Set(ctx, token, payloadData, time.Duration(10*time.Minute)).Err(); err != nil {
		panic(fmt.Sprintf("unable to save user access token for user %d , error: %s", userData.Id, err.Error()))
	}
	return token
}
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

func TestGetPrizeCategory(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Get("/prize_categories", GetPrizeCategory)

	// Initialize the assert object
	a := assert.New(t)

	// Run the test
	req := httptest.NewRequest("GET", "/prize_categories", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", access_token)

	resp, _ := app.Test(req, -1)
	// Check the status code
	a.Equal(fiber.StatusOK, resp.StatusCode)

	// Read the response body
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	// Check each field
	if resp.StatusCode == 200 {
		singleRecord, ok := result["data"].([]interface{})
		a.True(ok, "data should be an array")
		if len(singleRecord) != 0 {
			dataRecord, ok := singleRecord[0].(map[string]interface{})
			a.True(ok, "dataRecord should be a map")
			a.NotEmpty(dataRecord["id"], "id")
			a.NotEmpty(dataRecord["name"], "name")
			a.NotEmpty(dataRecord["status"], "status")
			a.NotEmpty(dataRecord["created_at"], "created_at")
		}
	}
}
func TestGetPrizeType(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Get("/prize_type/:prize_category?", GetPrizeType)

	// Initialize the assert object
	a := assert.New(t)

	// Run the test
	req := httptest.NewRequest("GET", "/prize_type", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", access_token)

	resp, _ := app.Test(req, -1)
	// Check the status code
	a.Equal(fiber.StatusOK, resp.StatusCode)

	// Read the response body
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	// Check each field
	if resp.StatusCode == 200 {
		singleRecord, ok := result["data"].([]interface{})
		a.True(ok, "data should be an array")
		if len(singleRecord) != 0 {
			dataRecord, ok := singleRecord[0].(map[string]interface{})
			a.True(ok, "dataRecord should be a map")
			a.NotEmpty(dataRecord["id"], "id")
			a.NotEmpty(dataRecord["name"], "name")
			a.NotEmpty(dataRecord["value"], "value")
			a.NotEmpty(dataRecord["elligibility"], "elligibility")
			a.NotEmpty(dataRecord["created_at"], "created_at")
			a.NotEmpty(dataRecord["status"], "status")
		}
	}
}
func TestGetPrizeTypeByCategory(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Get("/prize_type/:prize_category?", GetPrizeType)

	// Initialize the assert object
	a := assert.New(t)

	// Run the test
	req := httptest.NewRequest("GET", "/prize_type/1", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", access_token)

	resp, _ := app.Test(req, -1)
	// Check the status code
	a.Equal(fiber.StatusOK, resp.StatusCode)

	// Read the response body
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	// Check each field
	if resp.StatusCode == 200 {
		singleRecord, ok := result["data"].([]interface{})
		a.True(ok, "data should be an array")
		if len(singleRecord) == 0 {
			t.Fatal("prize type should not be empty")
		}
		dataRecord, ok := singleRecord[0].(map[string]interface{})
		a.True(ok, "dataRecord should be a map")
		a.NotEmpty(dataRecord["id"], "id")
		a.Equal(dataRecord["name"], "Test Prize 1", "name")
		a.NotEmpty(dataRecord["value"], "value")
		a.NotEmpty(dataRecord["elligibility"], "elligibility")
		a.NotEmpty(dataRecord["created_at"], "created_at")
		a.NotEmpty(dataRecord["status"], "status")
	}
}

func TestGetEntries(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Get("/entries", GetEntries)

	// Initialize the assert object
	a := assert.New(t)

	// Run the test
	req := httptest.NewRequest("GET", "/entries", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", access_token)

	resp, _ := app.Test(req, -1)
	// Check the status code
	a.Equal(fiber.StatusOK, resp.StatusCode)

	// Read the response body
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	// Check each field
	if resp.StatusCode == 200 {
		singleRecord, ok := result["data"].([]interface{})
		a.True(ok, "data should be an array")
		if len(singleRecord) != 0 {
			dataRecord, ok := singleRecord[0].(map[string]interface{})
			a.True(ok, "dataRecord should be a map")
			a.NotEmpty(dataRecord["id"], "id")
			a.NotEmpty(dataRecord["name"], "name")
			a.NotEmpty(dataRecord["code"], "code")
			a.NotEmpty(dataRecord["customer"], "customer")
			a.NotEmpty(dataRecord["created_at"], "created_at")
		}
	}
}
func TestGetDraws(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Get("/draws", GetDraws)

	// Initialize the assert object
	a := assert.New(t)

	// Run the test
	req := httptest.NewRequest("GET", "/draws", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", access_token)

	resp, _ := app.Test(req, -1)
	// Check the status code
	a.Equal(fiber.StatusOK, resp.StatusCode)

	// Read the response body
	body, _ := io.ReadAll(resp.Body)
	// fmt.Println(string(body))
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	// Check each field
	if resp.StatusCode == 200 {
		singleRecord, ok := result["data"].([]interface{})
		a.True(ok, "data should be an array")
		if len(singleRecord) != 0 {
			dataRecord, ok := singleRecord[0].(map[string]interface{})
			a.True(ok, "dataRecord should be a map")
			a.NotEmpty(dataRecord["id"], "id")
			a.NotEmpty(dataRecord["prize_type"], "prize_type")
			a.NotEmpty(dataRecord["code"], "code")
			a.NotEmpty(dataRecord["customer"], "customer")
			a.NotEmpty(dataRecord["created_at"], "created_at")
		}
	}
}
func TestGetPrizes(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Get("/prizes", GetPrizes)

	// Initialize the assert object
	a := assert.New(t)

	// Run the test
	req := httptest.NewRequest("GET", "/prizes", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", access_token)

	resp, _ := app.Test(req, -1)
	// Check the status code
	a.Equal(fiber.StatusOK, resp.StatusCode)

	// Read the response body
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	// Check each field
	if resp.StatusCode == 200 {
		singleRecord, ok := result["data"].([]interface{})
		a.True(ok, "data should be an array")
		if len(singleRecord) != 0 {
			dataRecord, ok := singleRecord[0].(map[string]interface{})
			a.True(ok, "dataRecord should be a map")
			a.NotEmpty(dataRecord["id"], "id")
			a.NotEmpty(dataRecord["value"], "value")
			a.NotEmpty(dataRecord["category"], "category")
			a.NotEmpty(dataRecord["customer"], "customer")
			a.NotEmpty(dataRecord["created_at"], "created_at")
		}
	}
}

func TestCreatePrizeCategory(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/prize_category", CreatePrizeCategory)
	uniqueName := fmt.Sprintf("CASH-%d", time.Now().Unix())
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]any
		expectedCode int
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description: "Success",
			payload: map[string]any{
				"name": uniqueName,
			},
			expectedCode: 200,
			expectedData: map[string]interface{}{
				"status":  200,
				"message": "Registration completed",
			},
		},
		{
			description: "Duplicate",
			payload: map[string]any{
				"name": uniqueName,
			},
			expectedCode: 409,
			expectedData: map[string]interface{}{
				"status":  409,
				"message": "duplicate",
			},
		},
		{
			description: "Invalid name",
			payload: map[string]any{
				"name": "CAR%as",
			},
			expectedCode: 406,
		},
		{
			description:  "required field",
			expectedCode: 400,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/prize_category", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", access_token)

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)
		// Read the response body
		// body, _ := io.ReadAll(resp.Body)
		// fmt.Println(string(body))
	}
}

func TestCreatePrizeType(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/prize_type", CreatePrizeType)
	uniqueName := fmt.Sprintf("CASH-%d", time.Now().Unix())
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]any
		expectedCode int
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description: "Success",
			payload: map[string]any{
				"name":         uniqueName,
				"category_id":  1,
				"value":        100,
				"elligibility": 1,
			},
			expectedCode: 200,
			expectedData: map[string]interface{}{
				"status":  200,
				"message": "success",
			},
		},
		{
			description: "Duplicate",
			payload: map[string]any{
				"name":         uniqueName,
				"category_id":  1,
				"value":        100,
				"elligibility": 1,
			},
			expectedCode: 409,
			expectedData: map[string]interface{}{
				"status":  409,
				"message": "duplicate",
			},
		},
		{
			description: "Invalid name",
			payload: map[string]any{
				"name":         "CAR%42",
				"category_id":  1,
				"value":        100,
				"elligibility": 1,
			},
			expectedCode: 406,
		},
		{
			description: "Invalid category id",
			payload: map[string]any{
				"name":         "CAR 42",
				"category_id":  -1,
				"value":        100,
				"elligibility": 1,
			},
			expectedCode: 406,
		},
		{
			description:  "required field",
			expectedCode: 400,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/prize_type", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", access_token)

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)
		// Read the response body
		// body, _ := io.ReadAll(resp.Body)
		// fmt.Println(string(body))
	}
}

func TestAddUser(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/user", AddUser)
	uniqueName := fmt.Sprintf("User %d", time.Now().Unix())
	uniqueEmail := fmt.Sprintf("user%d@example.com", time.Now().Unix())
	uniquePhone := fmt.Sprintf("078%s", fmt.Sprintf("%d", time.Now().Unix())[3:])
	fmt.Println(uniquePhone)
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]any
		expectedCode int
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description: "Success",
			payload: map[string]any{
				"fname":            "Test",
				"lname":            uniqueName,
				"email":            uniqueEmail,
				"phone":            uniquePhone,
				"department":       1,
				"can_add_codes":    true,
				"can_trigger_draw": true,
				"can_add_user":     true,
				"can_view_logs":    true,
			},
			expectedCode: 200,
			expectedData: map[string]interface{}{
				"status":  200,
				"message": "success",
			},
		},
		{
			description: "Duplicate email",
			payload: map[string]any{
				"fname":            "Test",
				"lname":            uniqueName,
				"email":            uniqueEmail,
				"phone":            "0782394234",
				"department":       1,
				"can_add_codes":    true,
				"can_trigger_draw": true,
				"can_add_user":     true,
				"can_view_logs":    true,
			},
			expectedCode: 409,
			expectedData: map[string]interface{}{
				"status":  409,
				"message": "duplicate",
			},
		},
		{
			description: "Duplicate phone",
			payload: map[string]any{
				"fname":            "Test",
				"lname":            uniqueName,
				"email":            "erfsad@asdfd.com",
				"phone":            uniquePhone,
				"department":       1,
				"can_add_codes":    true,
				"can_trigger_draw": true,
				"can_add_user":     true,
				"can_view_logs":    true,
			},
			expectedCode: 409,
			expectedData: map[string]interface{}{
				"status":  409,
				"message": "duplicate",
			},
		},
		{
			description: "Invalid name",
			payload: map[string]any{
				"fname":            "Test*sda",
				"lname":            "asd",
				"email":            "erfsad@asdfd.com",
				"phone":            "0788888121",
				"department":       1,
				"can_add_codes":    true,
				"can_trigger_draw": true,
				"can_add_user":     true,
				"can_view_logs":    true,
			},
			expectedCode: 406,
		},
		{
			description: "Invalid department id",
			payload: map[string]any{
				"fname":            "Test*sda",
				"lname":            "asd",
				"email":            "erfsad@asdfd.com",
				"phone":            "0788888121",
				"department":       -1,
				"can_add_codes":    true,
				"can_trigger_draw": true,
				"can_add_user":     true,
				"can_view_logs":    true,
			},
			expectedCode: 406,
		},
		{
			description:  "required field",
			expectedCode: 400,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/user", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", access_token)

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)
		// Read the response body
		// body, _ := io.ReadAll(resp.Body)
		// fmt.Println(string(body))
	}
}
func TestGetUsers(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Get("/users", GetUsers)

	// Initialize the assert object
	a := assert.New(t)

	// Run the test
	req := httptest.NewRequest("GET", "/users", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", access_token)

	resp, _ := app.Test(req, -1)
	// Check the status code
	a.Equal(fiber.StatusOK, resp.StatusCode)

	// Read the response body
	body, _ := io.ReadAll(resp.Body)
	// fmt.Println(string(body))
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	// Check each field
	if resp.StatusCode == 200 {
		singleRecord, ok := result["data"].([]interface{})
		a.True(ok, "data should be an array")
		if len(singleRecord) != 0 {
			dataRecord, ok := singleRecord[0].(map[string]interface{})
			a.True(ok, "dataRecord should be a map")
			a.NotEmpty(dataRecord["id"], "id")
			a.NotEmpty(dataRecord["firstname"], "firstname")
			a.NotEmpty(dataRecord["lastname"], "lastname")
			a.NotEmpty(dataRecord["email"], "email")
			a.NotEmpty(dataRecord["phone"], "phone")
			a.NotEmpty(dataRecord["department"], "department")
			a.NotEmpty(dataRecord["avatar_url"], "avatar_url")
			a.NotEmpty(dataRecord["can_add_codes"], "can_add_codes")
			a.NotEmpty(dataRecord["created_at"], "created_at")
		}
	}
}
