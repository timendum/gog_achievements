package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"golang.org/x/sys/windows/registry"
)

const (
	GalaxyClientID     = "46899977096215655"
	GalaxyClientSecret = "9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"
)

// getRefreshToken reads refreshToken from Windows registry
func getRefreshToken() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\GOG.com\Galaxy`, registry.QUERY_VALUE)
	if err != nil {
		logf("Failed to open registry key: %v", err)
		return "", err
	}
	defer key.Close()

	val, _, err := key.GetStringValue("refreshToken")
	if err != nil {
		logf("Failed to read refresh token from registry: %v", err)
		return "", err
	}
	logf("Successfully retrieved refresh token from registry: %s", val)
	return val, nil
}

// getAuth performs authentication using refresh token
func getAuth(refreshToken, clientID, clientSecret string) (*AuthResponse, error) {
	if clientID == "" {
		clientID = GalaxyClientID
	}
	if clientSecret == "" {
		clientSecret = GalaxyClientSecret
	}
	// Check config first
	cached, err := getAccessFromConfig(clientID)
	if err == nil && cached != nil {
		return cached, nil
	}

	logf("Fetching new access token from GOG...")
	params := url.Values{}
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", refreshToken)
	params.Set("client_id", clientID)
	params.Set("client_secret", clientSecret)
	params.Set("without_new_session", "1")

	resp, err := httpClient.Get("https://auth.gog.com/token?" + params.Encode())
	if err != nil {
		logf("Failed to connect to GOG auth server: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logf("Authentication failed with status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("authentication failed with status: %d", resp.StatusCode)
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		logf("Failed to decode authentication response: %v", err)
		return nil, err
	}

	saveAccessToConfig(clientID, &authResp)
	return &authResp, nil
}
