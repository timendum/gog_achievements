package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// GetAuthPath returns the path to auths.json
func GetAuthPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "auths.json"
	}
	return filepath.Join(filepath.Dir(exe), "auths.json")
}

// GetProductPath returns the path to products.json
func GetProductPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "products.json"
	}
	return filepath.Join(filepath.Dir(exe), "products.json")
}

// GetAccessFromConfig retrieves access token from config if not expired
func GetAccessFromConfig(clientID string) (*AuthResponse, error) {
	configPath := GetAuthPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			Logf("Config file does not exist: %s", configPath)
			return nil, nil
		}
		Logf("Failed to read config file: %v", err)
		return nil, err
	}

	var config map[string]AuthResponse
	if err := json.Unmarshal(data, &config); err != nil {
		Logf("Failed to unmarshal config: %v", err)
		return nil, err
	}

	authResp, exists := config[clientID]
	if !exists {
		return nil, nil
	}

	if authResp.ExpireTime == "" {
		Logf("Cached token has no expiration time")
		return nil, nil
	}

	expireTime, err := time.Parse(time.RFC3339, authResp.ExpireTime)
	if err != nil {
		Logf("Failed to parse expiration time: %v", err)
		return nil, nil
	}

	if time.Now().Before(expireTime) {
		Logf("Using cached access token (expires at %s)", authResp.ExpireTime)
		return &authResp, nil
	}

	Logf("Cached access token has expired")
	return nil, nil
}

// SaveAccessToConfig saves access token to config
func SaveAccessToConfig(clientID string, authResp *AuthResponse) error {
	configPath := GetAuthPath()
	config := make(map[string]AuthResponse)

	data, err := os.ReadFile(configPath)
	if err == nil {
		json.Unmarshal(data, &config)
	}

	now := time.Now()
	expireTime := now.Add(time.Duration(authResp.ExpiresIn) * time.Second)
	authResp.LoginTime = now.Format(time.RFC3339)
	authResp.ExpireTime = expireTime.Format(time.RFC3339)

	config[clientID] = *authResp

	data, err = json.MarshalIndent(config, "", "    ")
	if err != nil {
		Logf("Failed to marshal config: %v", err)
		return err
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		Logf("Failed to write config file: %v", err)
		return err
	}
	Logf("Updated cached auth for %s", clientID)
	return nil
}

// GetProductDataFromConfig retrieves access token from config if not expired
func GetProductDataFromConfig(productID int) (*ProductDetails, error) {
	configPath := GetProductPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			Logf("ProductData file does not exist: %s", configPath)
			return nil, nil
		}
		Logf("Failed to read config file: %v", err)
		return nil, err
	}

	var config map[int]ProductDetails
	if err := json.Unmarshal(data, &config); err != nil {
		Logf("Failed to unmarshal config: %v", err)
		return nil, err
	}

	productDetails, exists := config[productID]
	if !exists {
		return nil, nil
	}
	Logf("Using cached access product data for %s: client_id %s , client_secret %s", productID, productDetails.ClientID, productDetails.ClientSecret)
	return &productDetails, nil
}

func SaveProductDataToConfig(productID int, productDetails *ProductDetails) error {
	configPath := GetProductPath()
	config := make(map[int]ProductDetails)

	data, err := os.ReadFile(configPath)
	if err == nil {
		json.Unmarshal(data, &config)
	}

	config[productID] = *productDetails

	data, err = json.MarshalIndent(config, "", "    ")
	if err != nil {
		Logf("Failed to marshal config: %v", err)
		return err
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		Logf("Failed to write config file: %v", err)
		return err
	}
	Logf("Updated cached product data for %s", productID)
	return nil
}
