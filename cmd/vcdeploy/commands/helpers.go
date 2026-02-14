package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// truncate truncates a string to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// jsonReader wraps JSON data in an io.Reader.
func jsonReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}

// handleAPIError extracts and returns a formatted error from an API response.
func handleAPIError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("API error: %s (could not read response body)", resp.Status)
	}

	// Try to parse as JSON error
	var apiError struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}

	if json.Unmarshal(body, &apiError) == nil {
		if apiError.Error != "" {
			return fmt.Errorf("API error (%s): %s", resp.Status, apiError.Error)
		}
		if apiError.Message != "" {
			return fmt.Errorf("API error (%s): %s", resp.Status, apiError.Message)
		}
	}

	// Fall back to raw body
	if len(body) > 0 {
		return fmt.Errorf("API error: %s - %s", resp.Status, string(body))
	}

	return fmt.Errorf("API error: %s", resp.Status)
}
