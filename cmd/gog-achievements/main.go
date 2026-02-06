package main

import (
	"fmt"
	"time"

	"github.com/timendum/gog-achievements/internal"

	"github.com/alecthomas/kong"
)

func listAchievements(gameId string, authResp internal.AuthResponse) {
	// With game ID only: list achievements
	internal.Logf("Listing achievements mode for game: %s", gameId)
	achievements, err := internal.GetAchievements(gameId, authResp.UserID, authResp.AccessToken)
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

	internal.Logf("Summary: %d unlocked, %d locked", unlockedCount, lockedCount)
}

func listGames(authResp internal.AuthResponse) {
	owned := internal.ListOwnedGameIDs(authResp)
	if owned == nil {
		return
	}
	for _, productID := range *owned {
		gameDetail := internal.GetGameDetail(productID)
		if len(gameDetail.Builds) == 0 {
			internal.Logf("- %d is empty, skipped: %s", productID, gameDetail.Title)
			continue
		}
		fmt.Printf("- %d: %s\n", productID, gameDetail.Title)
	}
}

func main() {
	var cli struct {
		GameID         string   `arg:"" optional:"" help:"Game ID (leave empty to list owned games)"`
		AchievementIDs []string `arg:"" name:"achievement-id" optional:"" help:"Achievement IDs to unlock (requires GameID), leave empty to list game achievements"`
		Verbose        bool     `short:"v" help:"Enable verbose logging"`
		Clear          bool     `short:"c" help:"Clear (delete) achievements"`
	}

	ctx := kong.Parse(&cli,
		kong.Name("gog-achievements"),
		kong.Description("An app to manage GOG Achievements:"+
			"\n\n- Without arguments, the app will list all games you own."+
			"\n\n- With a <game-id> the app will list all achievements of the game."+
			"\n\n- With a <game-id> and at least one <achievement-id>, the app will unlock the achievements."+
			"\n\n- With a <game-id>, flag '-c' and at least one <achievement-id>, the app will clear (lock) the achievements."),
		kong.UsageOnError(),
	)
	internal.Verbose = cli.Verbose

	refreshToken, err := internal.GetRefreshToken()
	if err != nil {
		fmt.Printf("Failed to get refresh token: %v\n", err)
		return
	}

	if refreshToken == "" {
		fmt.Println("No refresh token found in registry.")
		return
	}

	authResp, err := internal.GetAuth(refreshToken, "", "")
	if err != nil {
		fmt.Printf("Authentication failed: %v\n", err)
		return
	}

	if authResp.RefreshToken == "" {
		fmt.Println("No refresh token in response")
		return
	}

	internal.Logf("Successfully authenticated, user ID: %s", authResp.UserID)

	// No arguments: retrieve owned games
	if cli.GameID == "" {
		internal.Logf("Listing owned games mode")
		listGames(*authResp)
		return
	}

	// With achievement ID: unlock achievement
	if len(cli.AchievementIDs) > 0 {
		now := time.Now().UTC().Add(-3 * time.Second)
		for _, AchievementID := range cli.AchievementIDs {
			dateUnlocked := &now
			verb := "unlock"
			if cli.Clear {
				dateUnlocked = nil
				verb = "clear"
			}
			internal.Logf("Unlocking achievement mode for game: %s, achievement: %s", cli.GameID, AchievementID)
			err := internal.UnlockAchievement(cli.GameID, authResp.UserID, AchievementID, refreshToken, dateUnlocked)
			if err != nil {
				fmt.Printf("Failed to %s achievement: %v\n", verb, err)
				return
			}
			fmt.Printf("Achievement %s %sed successfully!\n", verb, AchievementID)
		}
		return
	}
	listAchievements(cli.GameID, *authResp)

	_ = ctx // silence unused variable warning
}
