package internal

// Achievement represents a GOG achievement
type Achievement struct {
	AchievementID          string  `json:"achievement_id"`
	AchievementKey         string  `json:"achievement_key"`
	Visible                bool    `json:"visible"`
	Name                   string  `json:"name"`
	Description            string  `json:"description"`
	ImageURLUnlocked       string  `json:"image_url_unlocked"`
	ImageURLLocked         string  `json:"image_url_locked"`
	Rarity                 float64 `json:"rarity"`
	DateUnlocked           *string `json:"date_unlocked"`
	RarityLevelDescription string  `json:"rarity_level_description"`
	RarityLevelSlug        string  `json:"rarity_level_slug"`
}

// AuthResponse represents the authentication response
type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	UserID       string `json:"user_id"`
	LoginTime    string `json:"login_time,omitempty"`
	ExpireTime   string `json:"expire_time,omitempty"`
}

// ProductData represents product information from GOGDB
type ProductData struct {
	Builds []Build `json:"builds"`
}

// Build represents a build from GOGDB
type Build struct {
	ID            int    `json:"id"`
	DatePublished string `json:"date_published"`
	Listed        bool   `json:"listed"`
}

// ProductDetails represents detailed build information
type ProductDetails struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// AchievementsResponse represents the API response for achievements
type AchievementsResponse struct {
	Items []Achievement `json:"items"`
}

// GetGamesResponse represents the list of owned games
type GetGamesResponse struct {
	Owned []int `json:"owned"`
}

// GameDetail represents detailed information about a game
type GameDetail struct {
	Title  string `json:"title"`
	Type   string `json:"type"`
	Builds []struct {
		ID int64 `json:"id"`
	}
	ID                    int         `json:"id"`
	ImageBackground       string      `json:"image_background"`
	ImageBoxart           string      `json:"image_boxart"`
	ImageGalaxyBackground string      `json:"image_galaxy_background"`
	ImageIcon             string      `json:"image_icon"`
	ImageIconSquare       interface{} `json:"image_icon_square"`
	ImageLogo             string      `json:"image_logo"`
	IncludesGames         []int       `json:"includes_games"`
}
