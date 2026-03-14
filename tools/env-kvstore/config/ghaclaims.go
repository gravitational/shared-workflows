package config

import (
	"github.com/golang-jwt/jwt/v5"
)

// GHAClaims are extracted from the GitHub Actions JWT token and used to identify the
// session and to name the Secrets Manager secrets.
type GHAClaims struct {
	RunID       string `json:"run_id"`
	SHA         string `json:"sha"`
	Repository  string `json:"repository"`
	Enterprise  string `json:"enterprise"`
	Environment string `json:"environment,omitempty"`
	jwt.RegisteredClaims
}
