package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// Using MockStorage from cache_test.go

func TestGetProviderVersion(t *testing.T) {
	// Set up test cases
	tests := []struct {
		name           string
		setupMock      func(*MockStorage)
		version        string
		expectedStatus int
		shouldError    bool
		expectedBody   map[string]interface{}
	}{
		{
			name: "successful response",
			setupMock: func(ms *MockStorage) {
				// No storage calls expected for this test case
			},
			version:        "1.4.1",
			expectedStatus: http.StatusOK,
			shouldError:    false,
			expectedBody: map[string]interface{}{
				"archives": map[string]interface{}{
					"darwin_arm64": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_darwin_arm64.zip",
					},
					"linux_amd64": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_linux_amd64.zip",
					},
					"linux_arm": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_linux_arm.zip",
					},
					"freebsd_386": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_freebsd_386.zip",
					},
					"windows_amd64": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_windows_amd64.zip",
					},
					"freebsd_arm": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_freebsd_arm.zip",
					},
					"freebsd_amd64": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_freebsd_amd64.zip",
					},
					"linux_arm64": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_linux_arm64.zip",
					},
					"linux_386": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_linux_386.zip",
					},
					"freebsd_arm64": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_freebsd_arm64.zip",
					},
					"darwin_amd64": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_darwin_amd64.zip",
					},
					"windows_386": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_windows_386.zip",
					},
					"windows_arm": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_windows_arm.zip",
					},
					"windows_arm64": map[string]interface{}{
						"url": "terraform-provider-onepassword_1.4.1_windows_arm64.zip",
					},
				},
			},
		},
		{
			name:           "version not found",
			setupMock:      func(ms *MockStorage) {},
			version:        "9.9.9",
			expectedStatus: http.StatusNotFound,
			shouldError:    true,
			expectedBody: map[string]interface{}{
				"error": "version not found",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up mock storage
			mockStorage := new(MockStorage)
			tc.setupMock(mockStorage)

			// Create handler with mock storage
			logger := logrus.New()
			handler := NewRegistryHandler(logger, mockStorage)

			// Create a response recorder
			w := httptest.NewRecorder()

			// Create a test context
			c, _ := gin.CreateTestContext(w)

			// Set up request parameters with the correct namespace and provider
			c.Params = gin.Params{
				{Key: "registry", Value: "registry.terraform.io"},
				{Key: "namespace", Value: "1Password"},
				{Key: "provider", Value: "onepassword"},
			}

			// Set up the request for the specific version
			c.Request, _ = http.NewRequest("GET", "/v1/providers/1Password/onepassword/versions", nil)
			// Set the version in the context as the route handler would
			c.Set("version", tc.version)

			// Create a test server that returns mock responses
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the request
				assert.Equal(t, "/v1/providers/1Password/onepassword/versions", r.URL.Path)
				assert.Equal(t, "Terraform/1.0.0", r.Header.Get("User-Agent"))

				// Log the request for debugging
				t.Logf("Mock server received request: %s %s", r.Method, r.URL.Path)

				// Return mock response with the requested version
				response := map[string]interface{}{
					"id": "1Password/onepassword",
					"versions": []map[string]interface{}{
						{
							"version":   "1.4.1",
							"protocols": []string{"5.0"},
							"platforms": []map[string]interface{}{
								{"os": "darwin", "arch": "arm64"},
								{"os": "linux", "arch": "amd64"},
								{"os": "linux", "arch": "arm"},
								{"os": "freebsd", "arch": "386"},
								{"os": "windows", "arch": "amd64"},
								{"os": "freebsd", "arch": "arm"},
								{"os": "freebsd", "arch": "amd64"},
								{"os": "linux", "arch": "arm64"},
								{"os": "linux", "arch": "386"},
								{"os": "freebsd", "arch": "arm64"},
								{"os": "darwin", "arch": "amd64"},
								{"os": "windows", "arch": "386"},
								{"os": "windows", "arch": "arm"},
								{"os": "windows", "arch": "arm64"},
							},
						},
					},
					"warnings": nil,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer testServer.Close()

			// Override the http client in the handler to use our test server
			handler.httpClient = testServer.Client()

			// Log the request details before calling the handler
			t.Logf("Request URL: %s", c.Request.URL.String())
			versionFromContext, exists := c.Get("version")
			t.Logf("Version from context: %v (exists: %v)", versionFromContext, exists)

			// Call the handler
			handler.GetProviderVersion(c)

			// Log the response for debugging
			t.Logf("Response status: %d", w.Code)
			t.Logf("Response body: %s", w.Body.String())

			// Check the response status code
			assert.Equal(t, tc.expectedStatus, w.Code)

			// Parse the response body
			var responseBody map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &responseBody)
			assert.NoError(t, err)

			// Verify the response
			if tc.shouldError {
				// For error cases, check the status code and error message
				assert.Equal(t, tc.expectedStatus, w.Code)
				if errors, exists := responseBody["error"]; exists {
					assert.Equal(t, tc.expectedBody["error"], errors)
				}
			} else {
				// For success cases, check the archives structure
				archives, ok := responseBody["archives"].(map[string]interface{})
				assert.True(t, ok, "archives should be a map")

				// Check that we have the expected platforms
				expectedArchives := tc.expectedBody["archives"].(map[string]interface{})
				for platform, expectedData := range expectedArchives {
					archiveData, exists := archives[platform].(map[string]interface{})
					assert.True(t, exists, "platform %s should exist in response", platform)
					if exists {
						expectedURL := expectedData.(map[string]interface{})["url"].(string)
						assert.Equal(t, expectedURL, archiveData["url"], "URL for %s does not match", platform)
					}
				}
			}
		})
	}
}

func TestGetProviderIndex(t *testing.T) {
	// Set up test cases
	type testCase struct {
		name           string
		setupMock      func(*MockStorage)
		expectedStatus int
		expectedBody   map[string]interface{}
		shouldError    bool
		registry      string
		namespace     string
		provider      string
	}

	tests := []testCase{
		{
			name: "successful response",
			setupMock: func(ms *MockStorage) {
				// No storage calls expected for this test case
			},
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"versions": map[string]interface{}{
					"1.2.3": struct{}{},
					"2.0.0": struct{}{},
				},
			},
			shouldError: false,
			registry:    "registry.terraform.io",
			namespace:   "1Password",
			provider:    "onepassword",
		},
		{
			name:           "invalid registry",
			setupMock:      func(ms *MockStorage) {},
			expectedStatus: http.StatusBadRequest,
			shouldError:    true,
			expectedBody: map[string]interface{}{
				"error": "invalid parameters",
			},
			registry:    "",
			namespace:   "",
			provider:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock storage
			mockStorage := new(MockStorage)
			tc.setupMock(mockStorage)

			// Create handler with mock storage
			logger := logrus.New()
			handler := NewRegistryHandler(logger, mockStorage)

			// Create a response recorder
			w := httptest.NewRecorder()

			// Create a test context
			c, _ := gin.CreateTestContext(w)

			// Set up request parameters based on test case
			c.Params = gin.Params{
				{Key: "registry", Value: tc.registry},
				{Key: "namespace", Value: tc.namespace},
				{Key: "provider", Value: tc.provider},
			}

			// Create a test server that returns mock responses
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the request
				expectedPath := "/v1/providers/1Password/onepassword"
				if tc.name == "invalid registry" {
					expectedPath = "/v1/providers///"
				}
				assert.Equal(t, expectedPath, r.URL.Path)
				assert.Equal(t, "Terraform/1.0.0", r.Header.Get("User-Agent"))

				if tc.name == "invalid registry" {
					http.Error(w, "invalid parameters", http.StatusBadRequest)
					return
				}

				// Return mock response matching the actual Terraform Registry API format
				response := map[string]interface{}{
					"id":           "1Password/onepassword/1.4.1",
					"owner":        "1Password",
					"namespace":    "1Password",
					"name":         "onepassword",
					"version":      "1.4.1",
					"tag":          "v1.4.1",
					"description":  "Terraform 1Password provider",
					"source":       "https://github.com/1Password/terraform-provider-onepassword",
					"published_at": "2023-01-01T00:00:00Z",
					"downloads":    10000,
					"tier":         "partner",
					"versions": []string{
						"1.0.0",
						"1.2.0",
						"1.4.1",
					},
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer testServer.Close()

			// Override the http client in the handler to use our test server
			handler.httpClient = testServer.Client()

			// Call the handler
			handler.GetProviderIndex(c)

			// Check the response
			assert.Equal(t, tc.expectedStatus, w.Code)

			// Parse the response body
			var responseBody map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &responseBody)
			assert.NoError(t, err)

			// Verify the response body matches expected
			if tc.name == "successful response" {
				// Check that versions is a map
				versions, ok := responseBody["versions"].(map[string]interface{})
				assert.True(t, ok, "versions should be a map")

				// Check that we have some versions
				assert.Greater(t, len(versions), 0, "should have at least one version")

				// Check that all values are empty structs
				for _, v := range versions {
					assert.Equal(t, map[string]interface{}{}, v, "version value should be an empty object")
				}
			} else {
				// For error cases, just check the error message
				assert.Equal(t, tc.expectedBody["error"], responseBody["error"])
			}
		})
	}
}
