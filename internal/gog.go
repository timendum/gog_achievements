package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// getProductData retrieves product data from GOGDB
func getProductData(productID string) (string, string, error) {
	cached, err := GetProductDataFromConfig(productID)
	if err == nil && cached != nil {
		return cached.ClientID, cached.ClientSecret, nil
	}
	resp, err := httpClient.Get(fmt.Sprintf("https://www.gogdb.org/data/products/%s/product.json", productID))
	if err != nil {
		Logf("Failed to fetch product data: %v", err)
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		Logf("Failed to get product data - status code: %d", resp.StatusCode)
		return "", "", fmt.Errorf("failed to get product data: %d", resp.StatusCode)
	}

	var productData ProductData
	if err := json.NewDecoder(resp.Body).Decode(&productData); err != nil {
		Logf("Failed to decode product data: %v", err)
		return "", "", err
	}

	// Filter builds
	var validBuilds []Build
	for _, build := range productData.Builds {
		if build.DatePublished != "" && build.Listed {
			validBuilds = append(validBuilds, build)
		}
	}

	if len(validBuilds) == 0 {
		Logf("No valid builds found for product ID: %s", productID)
		return "", "", fmt.Errorf("no valid builds found")
	}

	// Sort by date published (descending)
	sort.Slice(validBuilds, func(i, j int) bool {
		return validBuilds[i].DatePublished > validBuilds[j].DatePublished
	})

	latestBuildID := validBuilds[0].ID
	Logf("Found latest build ID: %d (published: %s)", latestBuildID, validBuilds[0].DatePublished)

	// Get build details
	resp, err = httpClient.Get(fmt.Sprintf("https://www.gogdb.org/data/products/%s/builds/%d.json", productID, latestBuildID))
	if err != nil {
		Logf("Failed to fetch build details: %v", err)
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		Logf("Failed to get build details - status code: %d", resp.StatusCode)
		return "", "", fmt.Errorf("failed to get build details: %d", resp.StatusCode)
	}

	var buildDetails ProductDetails
	if err := json.NewDecoder(resp.Body).Decode(&buildDetails); err != nil {
		Logf("Failed to decode build details: %v", err)
		return "", "", err
	}

	Logf("Successfully retrieved client ID and secret")

	SaveProductDataToConfig(productID, &buildDetails)
	return buildDetails.ClientID, buildDetails.ClientSecret, nil
}

// getAchievements retrieves achievements for a product
func GetAchievements(productID string, userID, accessToken string) ([]Achievement, error) {
	Logf("Fetching achievements for product ID: %s, user ID: %s", productID, userID)
	clientID, _, err := getProductData(productID)
	if err != nil {
		Logf("Failed to get product data: %v", err)
		return nil, err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://gameplay.gog.com/clients/%s/users/%s/achievements", clientID, userID), nil)
	if err != nil {
		Logf("Failed to create request: %v", err)
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Gog-Lc", "en")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		Logf("Failed to fetch achievements: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		Logf("Failed to get achievements - status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("failed to get achievements: %d", resp.StatusCode)
	}

	var achResp AchievementsResponse
	if err := json.NewDecoder(resp.Body).Decode(&achResp); err != nil {
		Logf("Failed to decode achievements response: %v", err)
		return nil, err
	}

	Logf("Successfully retrieved %d achievements", len(achResp.Items))
	return achResp.Items, nil
}

type AchievementReqBody struct {
	DateUnlocked *string `json:"date_unlocked"`
}

// unlockAchievement unlocks a specific achievement
func UnlockAchievement(productID string, userID, achievementID, refreshToken string, dateUnlocked *time.Time) error {
	Logf("Attempting to unlock achievement: %s for user: %s, product: %s", achievementID, userID, productID)

	clientID, clientSecret, err := getProductData(productID)
	if err != nil {
		Logf("Failed to get product data: %v", err)
		return err
	}

	authResp, err := GetAuth(refreshToken, clientID, clientSecret)
	if err != nil {
		Logf("Failed to authenticate: %v", err)
		return err
	}

	body := AchievementReqBody{
		DateUnlocked: nil,
	}

	if dateUnlocked != nil {
		dateUnlocked := dateUnlocked.Format("2006-01-02T15:04:05-0700")
		body.DateUnlocked = &dateUnlocked
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		Logf("Failed to marshal request body: %v", err)
		return err
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("https://gameplay.gog.com/clients/%s/users/%s/achievements/%s", clientID, userID, achievementID),
		strings.NewReader(string(bodyBytes)))
	if err != nil {
		Logf("Failed to create request: %v", err)
		return err
	}

	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		Logf("Failed to send unlock request: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		Logf("Failed to unlock achievement - status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("failed to unlock achievement: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func ListOwnedGameIDs(authResp AuthResponse) *[]int {
	req, err := http.NewRequest("GET", "https://embed.gog.com/user/data/games", nil)
	if err != nil {
		Logf("Failed to create request: %v", err)
		return nil
	}

	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		Logf("Failed to send getFilteredProducts request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		Logf("Failed to list games - status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
		return nil
	}
	var getGames GetGamesResponse
	if err := json.NewDecoder(resp.Body).Decode(&getGames); err != nil {
		Logf("Failed to decode list games response: %v", err)
		return nil
	}

	Logf("Successfully retrieved %d game ids", len(getGames.Owned))
	return &getGames.Owned
}

func GetGameDetail(productID int) *GameDetail {

	req, err := http.NewRequest("GET", fmt.Sprintf("https://www.gogdb.org/data/products/%d/product.json", productID), nil)
	if err != nil {
		Logf("Failed to create request: %v", err)
		return nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		Logf("Failed to send gameDetails for %d request: %v", productID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// Game not found, it happens :(
		Logf("Game %d not found", productID)
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		Logf("Failed to get game details for %d - status code: %d, response: %s", productID, resp.StatusCode, string(bodyBytes))
		return nil
	}
	var gameDetail GameDetail
	if err := json.NewDecoder(resp.Body).Decode(&gameDetail); err != nil {
		Logf("Failed to decode game Detail for %d response: %v", productID, err)
		return nil
	}
	return &gameDetail
}
