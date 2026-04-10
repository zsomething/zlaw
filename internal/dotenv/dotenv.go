// Package dotenv loads a .env file into the process environment.
// Existing env vars are never overwritten.
package dotenv

import (
	"os"

	"github.com/joho/godotenv"
)

// Load reads path and sets any unset environment variables found in it.
// Returns nil if the file does not exist.
func Load(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return godotenv.Load(path)
}

// LoadCwd loads .env from the current working directory.
func LoadCwd() error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	return Load(cwd + "/.env")
}
