package controller

import (
	"auth-service/config"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"shared-package/utils"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/h2non/gock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
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

func setupRouter() *fiber.App {
	app := fiber.New()
	app.Get("/login/google", LoginWithGoogle)
	app.Get("/auth/google/callback", GoogleCallback)
	app.Get("/login/github", LoginWithGithub)
	app.Get("/auth/github/callback", GithubCallback)
	return app
}

func TestLoginWithGoogle(t *testing.T) {
	app := setupRouter()

	req := httptest.NewRequest("GET", "/login/google", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("Expected status %d, got %d", http.StatusFound, resp.StatusCode)
	}

	redirectURL := resp.Header.Get("Location")
	expectedState := viper.GetString("saltKey")
	expectedURL := config.AppConfig.GoogleLoginConfig.AuthCodeURL(expectedState)

	if redirectURL != expectedURL {
		t.Fatalf("Expected redirect URL %s, got %s", expectedURL, redirectURL)
	}
}

func TestGoogleCallback(t *testing.T) {
	//mock google token exchange server
	defer gock.Off() // Flush pending mocks after test execution
	// gock.Observe(gock.DumpRequest)
	// Mock the OAuth 2.0 token endpoint
	gock.New("https://oauth2.googleapis.com").
		Post("/token").
		Reply(http.StatusOK).
		Body(bytes.NewBuffer([]byte(`{"access_token":"mock_access_token"}`)))

	// Mock the Fetch user data
	gock.New("https://www.googleapis.com").
		Get("/oauth2/v2/userinfo").
		Reply(http.StatusOK).
		Body(bytes.NewBuffer([]byte(`{
			"id": "112478960924057904803",
			"email": "test@qonics.com",
			"verified_email": true,
			"name": "SHEMA",
			"given_name": "DOE",
			"picture": "https://i.pravatar.cc/150?u=a042581f4e29026704d",
			"locale": "en"
		}`)))
	app := setupRouter()
	config.InitializeConfig()

	// Mock Google OAuth2 configuration
	oauth2Config := oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/auth/google/callback",
		Endpoint:     google.Endpoint,
	}

	config.AppConfig.GoogleLoginConfig = oauth2Config
	// Mock request with state and code
	req := httptest.NewRequest("GET", "/auth/google/callback?state=testSaltKey&code=test-code", nil)
	// req = req.WithContext(context.WithValue(req.Context(), oauth2.HTTPClient, httpClient))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body.Bytes(), &result)
	if _, ok := result["email"]; !ok {
		t.Fatalf("Invalid response, Email key not found")
	} else if _, ok := result["name"]; !ok {
		t.Fatalf("Invalid response, Name key not found")
	}
}

func TestLoginWithGithub(t *testing.T) {
	app := setupRouter()

	req := httptest.NewRequest("GET", "/login/github", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("Expected status %d, got %d", http.StatusFound, resp.StatusCode)
	}

	redirectURL := resp.Header.Get("Location")
	expectedState := viper.GetString("saltKey")
	expectedURL := config.AppConfig.GithubLoginConfig.AuthCodeURL(expectedState, oauth2.AccessTypeOffline)

	if redirectURL != expectedURL {
		t.Fatalf("Expected redirect URL %s, got %s", expectedURL, redirectURL)
	}
}

func TestGithubCallback(t *testing.T) {
	//mock google token exchange server
	defer gock.Off() // Flush pending mocks after test execution
	gock.Observe(gock.DumpRequest)
	// Mock the OAuth 2.0 token endpoint
	gock.New("https://github.com").
		Post("/login/oauth/access_token").
		Reply(http.StatusOK).
		Body(bytes.NewBuffer([]byte(`{"access_token":"mock_access_token"}`)))

	// Mock the Fetch user data
	gock.New("https://api.github.com").
		Get("/user").
		Reply(http.StatusOK).
		Body(bytes.NewBuffer([]byte(`{
			"id": "112478960924057904803",
			"email": "test@qonics.com",
			"company": "@qonics",
			"name": "SHEMA",
			"location": "Kigali - Rwanda",
			"avatar_url": "https://i.pravatar.cc/150?u=a042581f4e29026704d",
			"login": "testusername"
		}`)))
	app := setupRouter()
	config.InitializeConfig()

	// Mock Google OAuth2 configuration
	oauth2Config := oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/auth/github/callback",
		Endpoint:     github.Endpoint,
	}

	config.AppConfig.GoogleLoginConfig = oauth2Config
	// Mock request with state and code
	req := httptest.NewRequest("GET", "/auth/github/callback?state=testSaltKey&code=test-code", nil)
	// req = req.WithContext(context.WithValue(req.Context(), oauth2.HTTPClient, httpClient))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body.Bytes(), &result)
	if _, ok := result["email"]; !ok {
		t.Fatalf("Invalid response, Email key not found\nresponse: %s", body.String())
	} else if _, ok := result["name"]; !ok {
		t.Fatalf("Invalid response, Name key not found\nresponse: %s", body.String())
	} else if _, ok := result["login"]; !ok {
		t.Fatalf("Invalid response, Login key not found\nresponse: %s", body.String())
	}
}
