package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	githubactions "github.com/sethvargo/go-githubactions"
)

// releaseAPIURL is the Foundry VTT Package Release API endpoint.
//
// https://foundryvtt.com/article/package-release-api/
const releaseAPIURL = "https://foundryvtt.com/_api/packages/release_version/"

type releaseRequest struct {
	ID      string         `json:"id"`
	DryRun  bool           `json:"dry-run"`
	Release releasePayload `json:"release"`
}

type releasePayload struct {
	Version       string        `json:"version"`
	Manifest      string        `json:"manifest"`
	Notes         string        `json:"notes,omitempty"`
	Compatibility compatibility `json:"compatibility"`
}

type compatibility struct {
	Minimum  string `json:"minimum"`
	Verified string `json:"verified"`
	Maximum  string `json:"maximum,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

type releaseResponse struct {
	Status  string                `json:"status"`
	Page    string                `json:"page"`
	Message string                `json:"message"`
	Errors  map[string][]apiError `json:"errors"`
}

func main() {
	action := githubactions.New()
	client := &http.Client{Timeout: 30 * time.Second}
	if err := run(action, client, releaseAPIURL); err != nil {
		action.Fatalf("%s", err.Error())
	}
}

func run(action *githubactions.Action, client *http.Client, apiURL string) error {
	token := action.Getenv("FVTTP_TOKEN")
	if token == "" {
		return errors.New("missing environment variable 'FVTTP_TOKEN'")
	}

	id := action.GetInput("id")
	if id == "" {
		return errors.New("missing input 'id'")
	}

	version := action.GetInput("version")
	if version == "" {
		return errors.New("missing input 'version'")
	}

	manifest := action.GetInput("manifest")
	if manifest == "" {
		return errors.New("missing input 'manifest'")
	}

	compatMin := action.GetInput("compat_min")
	if compatMin == "" {
		return errors.New("missing input 'compat_min'")
	}

	compatVerified := action.GetInput("compat_verified")
	if compatVerified == "" {
		return errors.New("missing input 'compat_verified'")
	}

	dryRun := true
	if v := action.GetInput("dry_run"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid input 'dry_run' (must be a boolean): %w", err)
		}
		dryRun = parsed
	}

	reqBody := releaseRequest{
		ID:     id,
		DryRun: dryRun,
		Release: releasePayload{
			Version:  version,
			Manifest: manifest,
			Notes:    action.GetInput("notes"),
			Compatibility: compatibility{
				Minimum:  compatMin,
				Verified: compatVerified,
				Maximum:  action.GetInput("compat_max"),
			},
		},
	}

	result, statusCode, err := submitRelease(client, apiURL, token, reqBody)
	if err != nil {
		return err
	}

	if result.Status != "success" {
		return fmt.Errorf("package release request failed (status %d): %s", statusCode, formatAPIErrors(result.Errors))
	}

	if dryRun {
		action.Infof("Dry run succeeded: %s", result.Message)
	} else {
		action.Infof("Package release published: %s", result.Page)
	}

	action.SetOutput("status", result.Status)
	action.SetOutput("page", result.Page)
	action.SetOutput("message", result.Message)

	return nil
}

func submitRelease(client *http.Client, apiURL, token string, reqBody releaseRequest) (*releaseResponse, int, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to call Foundry package release API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, resp.StatusCode, fmt.Errorf("rate limited by Foundry package release API, retry after %s seconds", resp.Header.Get("Retry-After"))
	}

	var result releaseResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to parse response body (status %d): %s", resp.StatusCode, string(body))
	}

	return &result, resp.StatusCode, nil
}

func formatAPIErrors(errs map[string][]apiError) string {
	if len(errs) == 0 {
		return "unknown error"
	}

	parts := make([]string, 0, len(errs))
	for field, fieldErrors := range errs {
		for _, e := range fieldErrors {
			parts = append(parts, fmt.Sprintf("%s: %s (%s)", field, e.Message, e.Code))
		}
	}
	sort.Strings(parts)

	return strings.Join(parts, "; ")
}
