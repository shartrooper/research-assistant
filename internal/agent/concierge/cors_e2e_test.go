package concierge

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactHandler_CORS(t *testing.T) {
	// Setup temporary artifact directory
	tmpDir, err := os.MkdirTemp("", "artifacts-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test artifact
	testFileName := "test.txt"
	testContent := "hello world"
	err = os.WriteFile(filepath.Join(tmpDir, testFileName), []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	handler := NewArtifactHandler(tmpDir)
	// We need to strip the prefix in the test as well, or the handler should do it?
	// In main.go: http.StripPrefix("/artifacts/", fileServer).ServeHTTP(w, r)
	// Let's assume the handler expects the path relative to artifactDir,
	// or we wrap it in StripPrefix if needed.
	// For this test, let's wrap it to match production logic.
	mux := http.NewServeMux()
	mux.Handle("/artifacts/", http.StripPrefix("/artifacts/", handler))

	server := httptest.NewServer(mux)
	defer server.Close()

	tests := []struct {
		name           string
		method         string
		url            string
		headers        map[string]string
		expectedStatus int
		expectedOrigin string
		expectedBody   string
	}{
		{
			name:           "OPTIONS Preflight",
			method:         "OPTIONS",
			url:            server.URL + "/artifacts/test.txt",
			expectedStatus: http.StatusOK,
			expectedOrigin: "*",
		},
		{
			name:           "GET with Origin",
			method:         "GET",
			url:            server.URL + "/artifacts/test.txt",
			headers:        map[string]string{"Origin": "https://example.com"},
			expectedStatus: http.StatusOK,
			expectedOrigin: "*",
			expectedBody:   testContent,
		},
		{
			name:           "GET Non-existent File CORS",
			method:         "GET",
			url:            server.URL + "/artifacts/missing.txt",
			headers:        map[string]string{"Origin": "https://example.com"},
			expectedStatus: http.StatusNotFound,
			expectedOrigin: "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.url, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			origin := resp.Header.Get("Access-Control-Allow-Origin")
			if origin != tt.expectedOrigin {
				t.Errorf("expected Access-Control-Allow-Origin %q, got %q", tt.expectedOrigin, origin)
			}

			if tt.expectedBody != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}
				if string(body) != tt.expectedBody {
					t.Errorf("expected body %q, got %q", tt.expectedBody, string(body))
				}
			}
		})
	}
}
