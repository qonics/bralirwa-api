package controller

import (
	"context"
	"os"
	"testing"

	"libs/shared-package/proto"

	"github.com/stretchr/testify/assert"
)

func TestLog(t *testing.T) {
	// Setup the server
	server := &Server{}

	// Mock context
	ctx := context.Background()

	// Test cases
	tests := []struct {
		description    string
		request        *proto.LogRequest
		expectedResult string
		setup          func()
	}{
		{
			description: "log in terminal environment",
			request: &proto.LogRequest{
				Message:     "Terminal Test",
				LogLevel:    "info",
				ServiceName: "test-service",
				Identifier:  "123456",
			},
			expectedResult: "success",
			setup: func() {
				// Mock os.Stderr to simulate terminal
				originalStderr := os.Stderr
				defer func() { os.Stderr = originalStderr }()
				_, w, _ := os.Pipe()
				os.Stderr = w
			},
		},
		{
			description: "log in non-terminal environment",
			request: &proto.LogRequest{
				Message:     "Non-Terminal Test",
				LogLevel:    "debug",
				ServiceName: "test-service",
				Identifier:  "654321",
			},
			expectedResult: "success",
			setup: func() {
				// Ensure os.Stderr is not a terminal
				originalStderr := os.Stderr
				defer func() { os.Stderr = originalStderr }()
				_, w, _ := os.Pipe()
				os.Stderr = w
			},
		},
	}

	// Run test cases
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			test.setup()
			resp, err := server.Log(ctx, test.request)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedResult, resp.Response)
		})
	}
}
