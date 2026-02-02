package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kong"
)

var verbose bool

// logf logs a message if verbose flag is set
func logf(format string, args ...interface{}) {
	if verbose {
		fmt.Printf("[LOG] "+format+"\n", args...)
	}
}

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// getProductData retrieves product data from GOGDB
func getProductData(productID string) (string, string, error) {
	cached, err := getProductDataFromConfig(productID)
	if err == nil && cached != nil {
		return cached.ClientID, cached.ClientSecret, nil
	}
	resp, err := httpClient.Get(fmt.Sprintf("https://www.gogdb.org/data/products/%s/product.json", productID))
	if err != nil {
		logf("Failed to fetch product data: %v", err)
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logf("Failed to get product data - status code: %d", resp.StatusCode)
		return "", "", fmt.Errorf("failed to get product data: %d", resp.StatusCode)
	}

	var productData ProductData
	if err := json.NewDecoder(resp.Body).Decode(&productData); err != nil {
		logf("Failed to decode product data: %v", err)
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
		logf("No valid builds found for product ID: %s", productID)
		return "", "", fmt.Errorf("no valid builds found")
	}

	// Sort by date published (descending)
	sort.Slice(validBuilds, func(i, j int) bool {
		return validBuilds[i].DatePublished > validBuilds[j].DatePublished
	})

	latestBuildID := validBuilds[0].ID
	logf("Found latest build ID: %d (published: %s)", latestBuildID, validBuilds[0].DatePublished)

	// Get build details
	resp, err = httpClient.Get(fmt.Sprintf("https://www.gogdb.org/data/products/%s/builds/%d.json", productID, latestBuildID))
	if err != nil {
		logf("Failed to fetch build details: %v", err)
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		logf("Failed to get build details - status code: %d", resp.StatusCode)
		return "", "", fmt.Errorf("failed to get build details: %d", resp.StatusCode)
	}

	var buildDetails ProductDetails
	if err := json.NewDecoder(resp.Body).Decode(&buildDetails); err != nil {
		logf("Failed to decode build details: %v", err)
		return "", "", err
	}

	logf("Successfully retrieved client ID and secret")

	saveProductDataToConfig(productID, &buildDetails)
	return buildDetails.ClientID, buildDetails.ClientSecret, nil
}

// getAchievements retrieves achievements for a product
func getAchievements(productID, userID, accessToken string) ([]Achievement, error) {
	logf("Fetching achievements for product ID: %s, user ID: %s", productID, userID)
	clientID, _, err := getProductData(productID)
	if err != nil {
		logf("Failed to get product data: %v", err)
		return nil, err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://gameplay.gog.com/clients/%s/users/%s/achievements", clientID, userID), nil)
	if err != nil {
		logf("Failed to create request: %v", err)
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Gog-Lc", "en")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		logf("Failed to fetch achievements: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logf("Failed to get achievements - status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("failed to get achievements: %d", resp.StatusCode)
	}

	var achResp AchievementsResponse
	if err := json.NewDecoder(resp.Body).Decode(&achResp); err != nil {
		logf("Failed to decode achievements response: %v", err)
		return nil, err
	}

	logf("Successfully retrieved %d achievements", len(achResp.Items))
	return achResp.Items, nil
}

// unlockAchievement unlocks a specific achievement
func unlockAchievement(productID, userID, achievementID, refreshToken string, dateUnlocked *time.Time) error {
	logf("Attempting to unlock achievement: %s for user: %s, product: %s", achievementID, userID, productID)
	if dateUnlocked == nil {
		now := time.Now().UTC().Add(-3 * time.Second)
		dateUnlocked = &now
	}

	clientID, clientSecret, err := getProductData(productID)
	if err != nil {
		logf("Failed to get product data: %v", err)
		return err
	}

	authResp, err := getAuth(refreshToken, clientID, clientSecret)
	if err != nil {
		logf("Failed to authenticate: %v", err)
		return err
	}

	body := map[string]string{
		"date_unlocked": dateUnlocked.Format("2006-01-02T15:04:05-0700"),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		logf("Failed to marshal request body: %v", err)
		return err
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("https://gameplay.gog.com/clients/%s/users/%s/achievements/%s", clientID, userID, achievementID),
		strings.NewReader(string(bodyBytes)))
	if err != nil {
		logf("Failed to create request: %v", err)
		return err
	}

	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		logf("Failed to send unlock request: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logf("Failed to unlock achievement - status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("failed to unlock achievement: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func listAchievements(gameId string, authResp AuthResponse) {
	// With game ID only: list achievements
	logf("Listing achievements mode for game: %s", gameId)
	achievements, err := getAchievements(gameId, authResp.UserID, authResp.AccessToken)
	if err != nil {
		fmt.Printf("Failed to get achievements: %v\n", err)
		return
	}

	unlockedCount := 0
	lockedCount := 0

	fmt.Println("Unlocked Achievements:")
	for _, ach := range achievements {
		if ach.DateUnlocked != nil {
			fmt.Printf("- %s: %s = %s\n", ach.AchievementID, ach.Name, ach.Description)
			unlockedCount++
		}
	}

	fmt.Println("\nTo be unlocked Achievements:")
	for _, ach := range achievements {
		if ach.DateUnlocked == nil {
			fmt.Printf("- %s: %s = %s\n", ach.AchievementID, ach.Name, ach.Description)
			lockedCount++
		}
	}

	logf("Summary: %d unlocked, %d locked", unlockedCount, lockedCount)
}

func listGames(authResp AuthResponse) {
	req, err := http.NewRequest("GET", "https://embed.gog.com/user/data/games", nil)
	if err != nil {
		logf("Failed to create request: %v", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		logf("Failed to send getFilteredProducts request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logf("Failed to list games - status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
		return
	}
	var getGames GetGamesResponse
	if err := json.NewDecoder(resp.Body).Decode(&getGames); err != nil {
		logf("Failed to decode list games response: %v", err)
		return
	}

	logf("Successfully retrieved %d game ids", len(getGames.Owned))
	for _, productID := range getGames.Owned {
		req, err = http.NewRequest("GET", fmt.Sprintf("https://www.gogdb.org/data/products/%d/product.json", productID), nil)
		if err != nil {
			logf("Failed to create request: %v", err)
			return
		}
		resp, err = httpClient.Do(req)
		if err != nil {
			logf("Failed to send gameDetails for %d request: %v", productID, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			// Game not found, it happens :(
			logf("Game %d not found", productID)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			logf("Failed to get game details for %d - status code: %d, response: %s", productID, resp.StatusCode, string(bodyBytes))
			return
		}
		var gameDetail GameDetail
		if err := json.NewDecoder(resp.Body).Decode(&gameDetail); err != nil {
			logf("Failed to decode game Detail for %d response: %v", productID, err)
			return
		}
		if len(gameDetail.Builds) == 0 {
			logf("- %d is empty, skipped: %s", productID, gameDetail.Title)
			continue
		}
		fmt.Printf("- %d: %s\n", productID, gameDetail.Title)
	}
}

func main() {
	var cli struct {
		GameID        string `arg:"" optional:"" help:"Game ID (leave empty to list owned games)"`
		AchievementID string `arg:"" optional:"" help:"Achievement ID to unlock (requires GameID), leave empty to list game achievements"`
		Verbose       bool   `flag:"-v" help:"Enable verbose logging"`
	}

	ctx := kong.Parse(&cli)
	verbose = cli.Verbose

	refreshToken, err := getRefreshToken()
	if err != nil {
		fmt.Printf("Failed to get refresh token: %v\n", err)
		return
	}

	if refreshToken == "" {
		fmt.Println("No refresh token found in registry.")
		return
	}

	authResp, err := getAuth(refreshToken, "", "")
	if err != nil {
		fmt.Printf("Authentication failed: %v\n", err)
		return
	}

	if authResp.RefreshToken == "" {
		fmt.Println("No refresh token in response")
		return
	}

	logf("Successfully authenticated, user ID: %s", authResp.UserID)

	// No arguments: retrieve owned games
	if cli.GameID == "" {
		logf("Listing owned games mode")
		listGames(*authResp)
		return
	}

	// With achievement ID: unlock achievement
	if cli.AchievementID != "" {
		logf("Unlocking achievement mode for game: %s, achievement: %s", cli.GameID, cli.AchievementID)
		err := unlockAchievement(cli.GameID, authResp.UserID, cli.AchievementID, refreshToken, nil)
		if err != nil {
			fmt.Printf("Failed to unlock achievement: %v\n", err)
			return
		}
		fmt.Println("Achievement unlocked successfully!")
		return
	}
	listAchievements(cli.GameID, *authResp)

	_ = ctx // silence unused variable warning
}
