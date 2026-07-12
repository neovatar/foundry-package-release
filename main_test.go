package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	githubactions "github.com/sethvargo/go-githubactions"
)

// newTestAction builds an Action whose inputs/env come from the given map
// and whose stdout/output-file writes can be inspected, so run() can be
// exercised without touching the real environment.
func newTestAction(t *testing.T, env map[string]string) (action *githubactions.Action, stdout *strings.Builder, outputFile string) {
	t.Helper()

	outputFile = filepath.Join(t.TempDir(), "github_output")
	if err := os.WriteFile(outputFile, nil, 0o600); err != nil {
		t.Fatalf("failed to create fake GITHUB_OUTPUT file: %v", err)
	}

	getenv := func(key string) string {
		if key == "GITHUB_OUTPUT" {
			return outputFile
		}
		return env[key]
	}

	stdout = &strings.Builder{}
	action = githubactions.New(
		githubactions.WithGetenv(getenv),
		githubactions.WithWriter(stdout),
	)

	return action, stdout, outputFile
}

func validInputs() map[string]string {
	return map[string]string{
		"FVTTP_TOKEN":           "fvttp_test-token",
		"INPUT_ID":              "example-module",
		"INPUT_VERSION":         "1.0.0",
		"INPUT_MANIFEST":        "https://example.com/releases/1.0.0/module.json",
		"INPUT_NOTES":           "https://example.com/releases/1.0.0",
		"INPUT_COMPAT_MIN":      "10.312",
		"INPUT_COMPAT_VERIFIED": "12",
		"INPUT_COMPAT_MAX":      "12.999",
	}
}

func TestRun_Success(t *testing.T) {
	var gotReq releaseRequest
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		json.NewEncoder(w).Encode(releaseResponse{
			Status:  "success",
			Page:    "https://foundryvtt.com/packages/example-module/edit/",
			Message: "Dry run completed successfully. To save, submit the request again without dry-run",
		})
	}))
	defer server.Close()

	action, stdout, outputFile := newTestAction(t, validInputs())

	if err := run(action, server.Client(), server.URL); err != nil {
		t.Fatalf("run() returned unexpected error: %v", err)
	}

	if gotAuth != "fvttp_test-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "fvttp_test-token")
	}

	if gotReq.ID != "example-module" {
		t.Errorf("request id = %q, want %q", gotReq.ID, "example-module")
	}
	if !gotReq.DryRun {
		t.Error("request dry-run = false, want true (default)")
	}
	if gotReq.Release.Version != "1.0.0" {
		t.Errorf("request version = %q, want %q", gotReq.Release.Version, "1.0.0")
	}
	if gotReq.Release.Compatibility.Minimum != "10.312" || gotReq.Release.Compatibility.Verified != "12" || gotReq.Release.Compatibility.Maximum != "12.999" {
		t.Errorf("request compatibility = %+v, want minimum=10.312 verified=12 maximum=12.999", gotReq.Release.Compatibility)
	}

	if !strings.Contains(stdout.String(), "Dry run succeeded") {
		t.Errorf("stdout = %q, want it to mention the dry run success", stdout.String())
	}

	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read GITHUB_OUTPUT file: %v", err)
	}
	if !strings.Contains(string(output), "status") || !strings.Contains(string(output), "success") {
		t.Errorf("output file = %q, want it to contain the 'status' output", output)
	}
	if !strings.Contains(string(output), "page") {
		t.Errorf("output file = %q, want it to contain the 'page' output", output)
	}
}

func TestRun_NotDryRun(t *testing.T) {
	var gotReq releaseRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotReq)
		json.NewEncoder(w).Encode(releaseResponse{
			Status: "success",
			Page:   "https://foundryvtt.com/packages/example-module/edit/",
		})
	}))
	defer server.Close()

	inputs := validInputs()
	inputs["INPUT_DRY_RUN"] = "false"
	action, stdout, _ := newTestAction(t, inputs)

	if err := run(action, server.Client(), server.URL); err != nil {
		t.Fatalf("run() returned unexpected error: %v", err)
	}

	if gotReq.DryRun {
		t.Error("request dry-run = true, want false")
	}
	if !strings.Contains(stdout.String(), "Package release published") {
		t.Errorf("stdout = %q, want it to mention the publish", stdout.String())
	}
}

func TestRun_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(releaseResponse{
			Status: "error",
			Errors: map[string][]apiError{
				"manifest": {{Message: "Enter a valid URL.", Code: "invalid"}},
			},
		})
	}))
	defer server.Close()

	action, _, _ := newTestAction(t, validInputs())

	err := run(action, server.Client(), server.URL)
	if err == nil {
		t.Fatal("run() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "Enter a valid URL") {
		t.Errorf("error = %v, want it to contain the API error message", err)
	}
}

func TestRun_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	action, _, _ := newTestAction(t, validInputs())

	err := run(action, server.Client(), server.URL)
	if err == nil {
		t.Fatal("run() expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "rate limited") || !strings.Contains(err.Error(), "30") {
		t.Errorf("error = %v, want it to mention rate limiting and the retry delay", err)
	}
}

func TestRun_MissingInputs(t *testing.T) {
	tests := []struct {
		name        string
		removeKey   string
		wantErrText string
	}{
		{"missing token", "FVTTP_TOKEN", "FVTTP_TOKEN"},
		{"missing id", "INPUT_ID", "id"},
		{"missing version", "INPUT_VERSION", "version"},
		{"missing manifest", "INPUT_MANIFEST", "manifest"},
		{"missing compat_min", "INPUT_COMPAT_MIN", "compat_min"},
		{"missing compat_verified", "INPUT_COMPAT_VERIFIED", "compat_verified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputs := validInputs()
			delete(inputs, tt.removeKey)
			action, _, _ := newTestAction(t, inputs)

			err := run(action, http.DefaultClient, "http://unused.invalid")
			if err == nil {
				t.Fatalf("run() expected an error for missing %q, got nil", tt.removeKey)
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantErrText)
			}
		})
	}
}

func TestRun_InvalidDryRunInput(t *testing.T) {
	inputs := validInputs()
	inputs["INPUT_DRY_RUN"] = "not-a-bool"
	action, _, _ := newTestAction(t, inputs)

	err := run(action, http.DefaultClient, "http://unused.invalid")
	if err == nil {
		t.Fatal("run() expected an error for invalid 'dry_run' input, got nil")
	}
	if !strings.Contains(err.Error(), "dry_run") {
		t.Errorf("error = %v, want it to mention 'dry_run'", err)
	}
}
