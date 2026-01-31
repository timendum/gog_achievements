# GOG Achievements Manager

This is a Go cli application allows you to manage GOG achievements manager.

Features:

- list all achievements, locked and unlocked
- unlock a locked achievement

## Requirements

- Go 1.21 or later
- Windows OS (uses Windows Registry for GOG Galaxy token)

## Installation

1. Install dependencies:

```bash
go mod download
```

1. Build the application:

```bash
go build -o gog-achievements.exe main.go
```

## Usage

### View all achievements for a game

```bash
./gog-achievements.exe <product_id>
# or
./gog-achievements.exe -product-id=<product_id>
```

### Unlock a specific achievement

```bash
./gog-achievements.exe <product_id> -a <achievement_id>
# or
./gog-achievements.exe -product-id=<product_id> -achievement-id=<achievement_id>
```

## Example

```bash
# List achievements for Hollow Knight: Silksong (product ID 1558393671)
./gog-achievements.exe 1558393671

# Unlock achievement Fanatic (ID 58889697238411342)
./gog-achievements.exe 1558393671 -a 58889697238411342
```

## How it works

1. Reads the GOG Galaxy refresh token from Windows Registry
2. Authenticates with GOG servers
3. Retrieves product data from GOGDB.org
4. Lists or unlocks achievements via GOG's gameplay API

## Notes

- API call results are cached in `auths.json` (for Auth) and `products.json` (for GOGDB) in the same directory as the executable
- GOG Galaxy refresh token is used for authentication, please verify that you are logged in.
