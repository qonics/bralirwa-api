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
	utils.IsTestMode = true
	utils.InitializeViper("config", "yml")
	viper.Set("encryption_key", "secret")
	config.InitializeConfig()
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
	_, err := config.DB.Exec(ctx, `INSERT INTO users (id,fname, lname, phone, email, can_add_codes,can_trigger_draw,can_add_user,can_view_logs, department_id, email_verified, phone_verified, locale, avatar_url, password, status, address, operator)
VALUES
(2, 'Admin', 'User test', '078234234232', 'test@qonics.com', true, true, true, true, 1, FALSE, FALSE, 'en', 'NOT_AVAILABLE',
 '$2a$06$Q3Omh4QCXB7f2a5mTqlrAunZgm3c4K1MZYraLh/OXgK43j8CoHyPa', 'OKAY', 'NOT_AVAILABLE', NULL);`)
	if err != nil {
		//update password to
		_, err = config.DB.Exec(ctx, `UPDATE users SET password=crypt($1, gen_salt('bf')) WHERE id=$2`, "P@as12.W0d", 2)
		if err != nil {
			fmt.Println("Error updating user password", err)
		}
		fmt.Println("Error inserting user data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO users (fname, lname, phone, email, can_add_codes,can_trigger_draw,can_add_user,can_view_logs, department_id, email_verified, phone_verified, locale, avatar_url, password, status, address, operator)
VALUES
('Admin', 'User test 2', '078234234231', 'test2@qonics.com', false, false, false, true, 1, true, true, 'en', 'NOT_AVAILABLE',
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
	_, err = config.DB.Exec(ctx, `INSERT INTO prize_type (id,name, prize_category_id, value, elligibility, status,period,trigger_by_system) VALUES (1,'Test Prize 1', 1, 100, 1, 'OKAY','DAILY',false);`)
	if err != nil {
		fmt.Println("Error inserting prize_type data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO prize_type (id,name, prize_category_id, value, elligibility, status,period,trigger_by_system) VALUES (2,'Test Prize 2', 1, 200, 5, 'OKAY','DAILY',true);`)
	if err != nil {
		fmt.Println("Error inserting prize_type data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO prize_type (id,name, prize_category_id, value, elligibility, status,period,trigger_by_system) VALUES (3,'Test Prize 3', 2, 100, 2, 'OKAY','WEEKLY',false);`)
	if err != nil {
		fmt.Println("Error inserting prize_type data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO prize_message (id, message, lang, prize_type_id,operator_id)
	VALUES (1, 'Congratulation, you won a prize','en',1, 1);
`)
	if err != nil {
		fmt.Println("Error inserting prize_message data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO customer (id, names,phone,phone_hash,province,district,locale,network_operator)
	VALUES (1, pgp_sym_encrypt('KALISA Doe', 'secret'),pgp_sym_encrypt('250785753712', 'secret')::bytea,digest('250785753712', 'sha256')::bytea,5,3,'en','MTN');
`)
	if err != nil {
		fmt.Println("Error inserting customer data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO codes (id, code, code_hash, prize_type_id,redeemed, status)
	VALUES (1, pgp_sym_encrypt('wrsdsad', 'secret')::bytea,digest('wrsdsad', 'sha256')::bytea, 1, false, 'OKAY');
`)
	if err != nil {
		fmt.Println("Error inserting code data", err)
	}
	_, err = config.DB.Exec(ctx, `INSERT INTO entries (id, customer_id,code_id)
	VALUES (1, 1, 1);
`)
	if err != nil {
		//update entry created_at to current date
		_, err = config.DB.Exec(ctx, `UPDATE entries SET created_at=$1 WHERE id=$2`, time.Now(), 1)
		fmt.Println("Error inserting entries data", err)
	}
	config.DB.Exec(ctx, `truncate draw CASCADE;`)
	config.DB.Exec(ctx, `truncate prize CASCADE;`)
}

func createTestAccessToken() string {
	userData := model.UserProfile{
		Id:             2,
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
				"password": "P@as12.W0d",
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

		if test.expectedData != nil {
			var result map[string]interface{}
			json.Unmarshal(body, &result)

			// Check detailed fields for successful login
			if resp.StatusCode == 200 {
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
			a.NotEmpty(dataRecord["messages"], "messages")
			a.NotEmpty(dataRecord["period"], "period")
			a.NotEmpty(dataRecord["distribution"], "distribution")
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
		a.NotEmpty(dataRecord["name"], "name")
		a.NotEmpty(dataRecord["value"], "value")
		a.NotEmpty(dataRecord["elligibility"], "elligibility")
		a.NotEmpty(dataRecord["created_at"], "created_at")
		a.NotEmpty(dataRecord["status"], "status")
		a.NotEmpty(dataRecord["messages"], "messages")
		a.NotEmpty(dataRecord["period"], "period")
		a.NotEmpty(dataRecord["distribution"], "distribution")
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
		expectedData map[string]any // expected data for detailed field checking
	}{
		{
			description: "Success",
			payload: map[string]any{
				"name":              uniqueName,
				"prize_category":    1,
				"value":             100,
				"elligibility":      1,
				"expiry_date":       "2025-11-29T22:00:00.000Z",
				"distribution":      "momo",
				"period":            "WEEKLY",
				"trigger_by_system": true,
				"messages": []map[string]any{
					{"message": "Congratulations, you have won a prize", "lang": "en"},
					{"message": "Mwatsindiye ibihembo", "lang": "rw"},
				},
			},
			expectedCode: 200,
			expectedData: map[string]any{
				"status":  200,
				"message": "success",
			},
		},
		{
			description: "Duplicate",
			payload: map[string]any{
				"name":              uniqueName,
				"prize_category":    1,
				"value":             100,
				"elligibility":      1,
				"expiry_date":       "2025-11-29T22:00:00.000Z",
				"distribution":      "momo",
				"period":            "WEEKLY",
				"trigger_by_system": true,
				"messages": []map[string]any{
					{"message": "Congratulations, you have won a prize", "lang": "en"},
					{"message": "Mwatsindiye ibihembo", "lang": "rw"},
				},
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
				"name":              "CAR%42",
				"prize_category":    1,
				"value":             100,
				"elligibility":      1,
				"expiry_date":       "2025-11-29T22:00:00.000Z",
				"distribution":      "momo",
				"period":            "WEEKLY",
				"trigger_by_system": true,
				"messages": []map[string]any{
					{"message": "Congratulations, you have won a prize", "lang": "en"},
					{"message": "Mwatsindiye ibihembo", "lang": "rw"},
				},
			},
			expectedCode: 406,
		},
		{
			description: "expired date",
			payload: map[string]any{
				"name":              "CAR-42",
				"prize_category":    1,
				"value":             100,
				"elligibility":      1,
				"expiry_date":       "2024-10-29T22:00:00.000Z",
				"distribution":      "momo",
				"period":            "WEEKLY",
				"trigger_by_system": true,
				"messages": []map[string]any{
					{"message": "Congratulations, you have won a prize", "lang": "en"},
					{"message": "Mwatsindiye ibihembo", "lang": "rw"},
				},
			},
			expectedCode: 406,
		},
		{
			description: "Invalid category id",
			payload: map[string]any{
				"name":              "CAR 42",
				"prize_category":    -1,
				"value":             100,
				"elligibility":      1,
				"expiry_date":       "2025-11-29T22:00:00.000Z",
				"distribution":      "momo",
				"period":            "WEEKLY",
				"trigger_by_system": true,
				"messages": []map[string]any{
					{"message": "Congratulations, you have won a prize", "lang": "en"},
					{"message": "Mwatsindiye ibihembo", "lang": "rw"},
				},
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
	uniquePhone := fmt.Sprintf("25078%s", fmt.Sprintf("%d", time.Now().Unix())[3:])
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
				"phone":            "250782394234",
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
				"phone":            "250788888121",
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
				"phone":            "250788888121",
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
			a.NotNil(dataRecord["can_add_codes"], "can_add_codes")
			a.NotEmpty(dataRecord["created_at"], "created_at")
		}
	}
}
func TestGetCustomer(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Get("/customer/:customerId", GetCustomer)

	// Initialize the assert object
	a := assert.New(t)

	// Run the test
	req := httptest.NewRequest("GET", "/customer/1", nil)
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
		dataRecord, ok := result["data"].(map[string]interface{})
		a.True(ok, "dataRecord should be a map")
		a.NotEmpty(dataRecord["id"], "id")
		a.NotEmpty(dataRecord["names"], "names")
		a.NotEmpty(dataRecord["province"], "province")
		a.NotEmpty(dataRecord["district"], "district")
		a.NotEmpty(dataRecord["phone"], "phone")
		a.NotEmpty(dataRecord["locale"], "locale")
		a.NotEmpty(dataRecord["created_at"], "created_at")
	}
}
func TestGetEntryData(t *testing.T) {
	access_token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Get("/entry/:entryId", GetEntryData)

	// Initialize the assert object
	a := assert.New(t)

	// Run the test
	req := httptest.NewRequest("GET", "/entry/1", nil)
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
		dataRecord, ok := result["data"].(map[string]interface{})
		a.True(ok, "dataRecord should be a map")
		a.NotEmpty(dataRecord["id"], "id")
		a.NotEmpty(dataRecord["customer"], "customer")
		a.NotEmpty(dataRecord["code"], "code")
		a.NotEmpty(dataRecord["created_at"], "created_at")
	}
}
func TestChangePassword(t *testing.T) {
	token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/change_password", ChangePassword)
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]string
		expectedCode int
		expectedBody string
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description: "Invalid existing password",
			payload: map[string]string{
				"current_password": "12345612",
				"new_password":     "P@as12.W0d2",
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description: "Same password as existing",
			payload: map[string]string{
				"current_password": "P@as12.W0d",
				"new_password":     "P@as12.W0d",
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description:  "missing fields",
			payload:      map[string]string{},
			expectedCode: 400,
		},
		{
			description: "Not strong password",
			payload: map[string]string{
				"current_password": "P@as12.W0d",
				"new_password":     "Pass@word",
			},
			expectedCode: fiber.StatusBadRequest,
		},
		{
			description: "Password changed successful",
			payload: map[string]string{
				"current_password": "P@as12.W0d",
				"new_password":     "P@as12.W0d2",
			},
			expectedCode: 200,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/change_password", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)

		// Read the response body
		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		// Check each field
		a.Equal(int(result["status"].(float64)), resp.StatusCode, test.description, "Status")
		a.NotEmpty(result["message"], test.description, "Message")
	}
}

var resetKey string

func TestForgotPassword(t *testing.T) {
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/forgot_password", ForgotPassword)
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]string
		expectedCode int
		expectedBody string
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description:  "missing fields",
			payload:      map[string]string{},
			expectedCode: 400,
		},
		{
			description: "Forgot Password done",
			payload: map[string]string{
				"email": "test2@qonics.com",
			},
			expectedCode: 200,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/forgot_password", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)

		// Read the response body
		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		// Check each field
		if resp.StatusCode == 200 {
			//set reset_key for next test (TestValidateOTP)
			resetKey = result["reset_key"].(string)
			a.NotEmpty(result["email"], test.description, "Email")
			a.NotEmpty(result["reset_key"], test.description, "Reset key")
		}
		a.Equal(int(result["status"].(float64)), resp.StatusCode, test.description, "Status")
		a.NotEmpty(result["message"], test.description, "Message")
	}
}

var resetPasswordKey string

func TestValidateOTP(t *testing.T) {
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/validate_otp", ValidateOTP)
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]string
		expectedCode int
		expectedBody string
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description:  "missing fields",
			payload:      map[string]string{},
			expectedCode: 400,
		},
		{
			description: "Invalid Reset key",
			payload: map[string]string{
				"otp":       "123456",
				"reset_key": "asdasdaasdas",
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description: "Invalid OTP",
			payload: map[string]string{
				"otp":       "000000",
				"reset_key": resetKey,
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description: "OTP validated",
			payload: map[string]string{
				"otp":       "123456",
				"reset_key": resetKey,
			},
			expectedCode: fiber.StatusOK,
		},
		{
			description: "Already used otp",
			payload: map[string]string{
				"otp":       "123456",
				"reset_key": resetKey,
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/validate_otp", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)

		// Read the response body
		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if resp.StatusCode == 200 {
			//set reset_key for next test (reset password)
			resetPasswordKey = result["reset_key"].(string)
			a.NotEmpty(result["email"], test.description, "Email")
			a.NotEmpty(result["reset_key"], test.description, "Reset key")
		}
		// Check each field
		a.Equal(int(result["status"].(float64)), resp.StatusCode, test.description, "Status")
		a.NotEmpty(result["message"], test.description, "Message")
	}
}
func TestSetNewPassword(t *testing.T) {
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/set_password", SetNewPassword)
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]string
		expectedCode int
		expectedBody string
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description:  "missing fields",
			payload:      map[string]string{},
			expectedCode: 400,
		},
		{
			description: "Invalid Reset key",
			payload: map[string]string{
				"password":  "123456",
				"reset_key": "asdasdaasdas",
			},
			expectedCode: fiber.StatusBadRequest,
		},
		{
			description: "Weak password",
			payload: map[string]string{
				"password":  "123456@s",
				"reset_key": resetPasswordKey,
			},
			expectedCode: fiber.StatusBadRequest,
		},
		{
			description: "password changed",
			payload: map[string]string{
				"password":  "P@as12.W0d",
				"reset_key": resetPasswordKey,
			},
			expectedCode: fiber.StatusOK,
		},
		{
			description: "Already used reset key",
			payload: map[string]string{
				"password":  "P@as12.W0d",
				"reset_key": resetPasswordKey,
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/set_password", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)

		// Read the response body
		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if resp.StatusCode == 200 {
			a.NotEmpty(result["email"], test.description, "Email")
		}
		// Check each field
		a.Equal(int(result["status"].(float64)), resp.StatusCode, test.description, "Status")
		a.NotEmpty(result["message"], test.description, "Message")
	}
}
func TestSendVerificationEmail(t *testing.T) {
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/send_verification_email", SendVerificationEmail)
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]string
		expectedCode int
		expectedBody string
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description:  "missing fields",
			payload:      map[string]string{},
			expectedCode: 400,
		},
		{
			description: "Verification email sent",
			payload: map[string]string{
				"email": "test@qonics.com",
			},
			expectedCode: 200,
		},
		{
			description: "Already verified email",
			payload: map[string]string{
				"email": "test2@qonics.com",
			},
			expectedCode: fiber.StatusAccepted,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/send_verification_email", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)

		// Read the response body
		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		// Check each field
		if resp.StatusCode == 200 {
			//set reset_key for next test (TestValidateOTP)
			resetKey = result["reset_key"].(string)
			a.NotEmpty(result["email"], test.description, "Email")
			a.NotEmpty(result["reset_key"], test.description, "Reset key")
		}
		a.Equal(int(result["status"].(float64)), resp.StatusCode, test.description, "Status")
		a.NotEmpty(result["message"], test.description, "Message")
	}
}

func TestVerifyOtp(t *testing.T) {
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/verify_otp", ValidateOTP)
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]string
		expectedCode int
		expectedBody string
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description:  "missing fields",
			payload:      map[string]string{},
			expectedCode: 400,
		},
		{
			description: "Invalid Reset key",
			payload: map[string]string{
				"otp":       "123456",
				"reset_key": "asdasdaasdas",
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description: "Invalid OTP",
			payload: map[string]string{
				"otp":       "000000",
				"reset_key": resetKey,
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description: "OTP validated",
			payload: map[string]string{
				"otp":       "123456",
				"reset_key": resetKey,
			},
			expectedCode: fiber.StatusOK,
		},
		{
			description: "Already used otp",
			payload: map[string]string{
				"otp":       "123456",
				"reset_key": resetKey,
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/verify_otp", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)

		// Read the response body
		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		if resp.StatusCode == 200 {
			//set reset_key for next test (reset password)
			a.NotEmpty(result["user_id"], test.description, "User id")
			a.NotEmpty(result["email"], test.description, "Email")
		}
		// Check each field
		a.Equal(int(result["status"].(float64)), resp.StatusCode, test.description, "Status")
		a.NotEmpty(result["message"], test.description, "Message")
	}
}
func TestStartPrizeDraw(t *testing.T) {
	token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/draw", StartPrizeDraw)
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]any
		expectedCode int
		expectedBody string
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description: "success",
			payload: map[string]any{
				"prize_type": 1,
			},
			expectedCode: fiber.StatusOK,
		},
		{
			description: "invalid prize type",
			payload: map[string]any{
				"prize_type": 1001,
			},
			expectedCode: fiber.StatusForbidden,
		},
		{
			description: "no entry found",
			payload: map[string]any{
				"prize_type": 3,
			},
			expectedCode: fiber.StatusExpectationFailed,
		},
		{
			description: "system draw",
			payload: map[string]any{
				"prize_type": 2,
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description:  "missing fields",
			payload:      map[string]any{},
			expectedCode: 400,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", "/draw", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)

		// Read the response body
		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		// Check each field
		a.Equal(int(result["status"].(float64)), resp.StatusCode, test.description, "Status")
		a.NotEmpty(result["message"], test.description, "Message")
		if resp.StatusCode == 200 {
			winner, ok := result["winner"].(map[string]interface{})
			if !ok {
				t.Errorf("Expected winner to be a map, but got %T", result["winner"])
			}
			a.NotEmpty(winner["prize_id"], test.description, "Prize type")
			a.NotEmpty(winner["draw_id"], test.description, "Draw id")
			a.NotEmpty(winner["winner"], test.description, "Winner")
			a.NotEmpty(winner["code"], test.description, "Code")
		}
	}
}

func TestChangeUserStatus(t *testing.T) {
	token := createTestAccessToken()
	// Setup Fiber app
	app := fiber.New()
	// Define the route
	app.Post("/change_user_status/:userId", ChangeUserStatus)
	// Test cases
	tests := []struct {
		description  string
		payload      map[string]any
		userId       int
		expectedCode int
		expectedBody string
		expectedData map[string]interface{} // expected data for detailed field checking
	}{
		{
			description: "success disable",
			userId:      2,
			payload: map[string]any{
				"status": "DISABLED",
			},
			expectedCode: fiber.StatusOK,
		},
		{
			description: "success enable",
			userId:      2,
			payload: map[string]any{
				"status": "OKAY",
			},
			expectedCode: fiber.StatusOK,
		},
		{
			description: "same status",
			userId:      2,
			payload: map[string]any{
				"status": "OKAY",
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description: "invalid user",
			userId:      -1,
			payload: map[string]any{
				"status": "OKAY",
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description: "invalid status",
			userId:      2,
			payload: map[string]any{
				"status": "OKAYY",
			},
			expectedCode: fiber.StatusNotAcceptable,
		},
		{
			description:  "missing fields",
			payload:      map[string]any{},
			expectedCode: 400,
		},
	}

	// Initialize the assert object
	a := assert.New(t)

	// Run the tests
	for _, test := range tests {
		reqBody, _ := json.Marshal(test.payload)
		req := httptest.NewRequest("POST", fmt.Sprintf("/change_user_status/%d", test.userId), bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)

		resp, _ := app.Test(req, -1)
		// Check the status code
		a.Equal(test.expectedCode, resp.StatusCode, test.description)

		// Read the response body
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		var result map[string]interface{}
		json.Unmarshal(body, &result)
		// Check each field
		a.Equal(int(result["status"].(float64)), resp.StatusCode, test.description, "Status")
		a.NotEmpty(result["message"], test.description, "Message")
	}
}
