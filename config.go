package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// getAuthPath returns the path to auths.json
func getAuthPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "auths.json"
	}
	return filepath.Join(filepath.Dir(exe), "auths.json")
}

// getProductPath returns the path to products.json
func getProductPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "products.json"
	}
	return filepath.Join(filepath.Dir(exe), "products.json")
}

// getAccessFromConfig retrieves access token from config if not expired
func getAccessFromConfig(clientID string) (*AuthResponse, error) {
	configPath := getAuthPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			logf("Config file does not exist: %s", configPath)
			return nil, nil
		}
		logf("Failed to read config file: %v", err)
		return nil, err
	}

	var config map[string]AuthResponse
	if err := json.Unmarshal(data, &config); err != nil {
		logf("Failed to unmarshal config: %v", err)
		return nil, err
	}

	authResp, exists := config[clientID]
	if !exists {
		return nil, nil
	}

	if authResp.ExpireTime == "" {
		logf("Cached token has no expiration time")
		return nil, nil
	}

	expireTime, err := time.Parse(time.RFC3339, authResp.ExpireTime)
	if err != nil {
		logf("Failed to parse expiration time: %v", err)
		return nil, nil
	}

	if time.Now().Before(expireTime) {
		logf("Using cached access token (expires at %s)", authResp.ExpireTime)
		return &authResp, nil
	}

	logf("Cached access token has expired")
	return nil, nil
}

// saveAccessToConfig saves access token to config
func saveAccessToConfig(clientID string, authResp *AuthResponse) error {
	configPath := getAuthPath()
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
		logf("Failed to marshal config: %v", err)
		return err
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		logf("Failed to write config file: %v", err)
		return err
	}
	logf("Updated cached auth for %s", clientID)
	return nil
}

// getAccessFromConfig retrieves access token from config if not expired
func getProductDataFromConfig(productID string) (*ProductDetails, error) {
	configPath := getProductPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			logf("ProductData file does not exist: %s", configPath)
			return nil, nil
		}
		logf("Failed to read config file: %v", err)
		return nil, err
	}

	var config map[string]ProductDetails
	if err := json.Unmarshal(data, &config); err != nil {
		logf("Failed to unmarshal config: %v", err)
		return nil, err
	}

	productDetails, exists := config[productID]
	if !exists {
		return nil, nil
	}
	logf("Using cached access product data for %s: client_id %s , client_secret %s", productID, productDetails.ClientID, productDetails.ClientSecret)
	return &productDetails, nil
}

func saveProductDataToConfig(productID string, productDetails *ProductDetails) error {
	configPath := getProductPath()
	config := make(map[string]ProductDetails)

	data, err := os.ReadFile(configPath)
	if err == nil {
		json.Unmarshal(data, &config)
	}

	config[productID] = *productDetails

	data, err = json.MarshalIndent(config, "", "    ")
	if err != nil {
		logf("Failed to marshal config: %v", err)
		return err
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		logf("Failed to write config file: %v", err)
		return err
	}
	logf("Updated cached product data for %s", productID)
	return nil
}
