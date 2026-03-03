package cognitotoken

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"

	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentity"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/golang-jwt/jwt/v5"
	"github.com/hashicorp/go-retryablehttp"
)

var (
	audience      = "cognito-identity.amazonaws.com"
	loginProvider = "token.actions.githubusercontent.com/teleport"

	minSessionDuration = 15 * time.Minute // matches Cognito's default
)

// CognitoGHATokenExchanger creates a role provider that exchanges a GitHub Actions JWT token for a Cognito OIDC token.
// The Cognito OIDC token can include claims that map to role session tags for IAM ABAC. Implements stscreds.IdentityTokenRetriever.
type CognitoGHATokenExchanger struct {
	Claims           GHAClaims
	ghaJWT           string
	cognitoOIDCToken string
	ctx				 context.Context

	gha     config.GHAConfig
	cognito config.CognitoConfig
}

// GHAClaims are extracted from the GitHub Actions JWT token and used to identify the
// session and to name the Secrets Manager secrets.
type GHAClaims struct {
	RunID       string `json:"run_id"`
	SHA         string `json:"sha"`
	Repository  string `json:"repository"`
	Enterprise  string `json:"enterprise"`
	Environment string `json:"environment"`
}

// NewTokenExchanger creates a new CognitoGHATokenExchanger with the provided Cognito and GHA configuration.
func NewTokenExchanger(ctx context.Context, cognitoConfig *config.CognitoConfig, ghaConfig *config.GHAConfig) *CognitoGHATokenExchanger {
	return &CognitoGHATokenExchanger{
		ctx:     ctx,
		ghaJWT:  ghaConfig.GitHubToken,
		gha:     *ghaConfig,
		cognito: *cognitoConfig,
	}
}

// GetIdentityToken satisfies the stscreds.WebIdentityRoleProvider interface.
// Attempts to retrieve a Cognito OIDC token, which will be cached for subsequent calls.
// If the GHA JWT token is not set, it will be retrieved and cached as well.
func (e *CognitoGHATokenExchanger) GetIdentityToken() ([]byte, error) {
	if e.cognitoOIDCToken == "" {
		if err := e.fetchCognitoOIDCToken(); err != nil {
			return nil, fmt.Errorf("error fetching Cognito OIDC token: %w", err)
		}
		if e.cognitoOIDCToken == "" {
			return nil, fmt.Errorf("received empty Cognito OIDC token")
		}
	}
	return []byte(e.cognitoOIDCToken), nil
}

// CreateProvider creates a new stscreds.WebIdentityRoleProvider that can be used to retrieve AWS credentials using the cached Cognito OIDC token.
func (e *CognitoGHATokenExchanger) CreateProvider() (*stscreds.WebIdentityRoleProvider, error) {
	sessionName, err := e.getAWSSessionName()
	if err != nil {
		return nil, fmt.Errorf("error getting AWS session name for Cognito role provider: %w", err)
	}

	provider := stscreds.NewWebIdentityRoleProvider(
		sts.New(sts.Options{Region: e.cognito.Region}),
		e.cognito.RoleARN,
		e,
		func(opt *stscreds.WebIdentityRoleOptions) {
			opt.RoleSessionName = sessionName
			opt.Duration = time.Duration(minSessionDuration) * time.Second
		},
	)

	return provider, nil
}

func (e *CognitoGHATokenExchanger) getAWSSessionName() (string, error) {
	if e.ghaJWT == "" {
		// if GHA JWT isn't set, we need to retrieve it to get the runID and SHA for the session name
		if err := e.fetchGHAJWT(); err != nil {
			return "", fmt.Errorf("cannot parse session name from JWT. error fetching GHA JWT token: %w", err)
		}
		if e.ghaJWT == "" {
			return "", fmt.Errorf("cannot parse session name from JWT. received empty GHA JWT token")
		}
	}

	claims := map[string]any{
		"run_id":      "",
		"sha":         "",
		"repository":  "",
		"enterprise":  "",
		"environment": "",
	}
	err := parseClaims("GHA", e.ghaJWT, &claims)
	if err != nil {
		return "", fmt.Errorf("error parsing GHA JWT claims: %w", err)
	}

	runID, okRunID := claims["run_id"].(string)
	sha, okSHA := claims["sha"].(string)
	if !okRunID || !okSHA || runID == "" || sha == "" {
		return "", fmt.Errorf("failed to extract run_id and sha from token claims")
	}

	e.Claims = GHAClaims{
		RunID:       runID,
		SHA:         sha,
		Repository:  claims["repository"].(string),
		Enterprise:  claims["enterprise"].(string),
		Environment: claims["environment"].(string),
	}

	sessionName := fmt.Sprintf("%s@%s", runID, sha)
	fmt.Printf("Using session name: %s\n", sessionName)
	return sessionName, nil
}

// fetchGHAJWT retrieves a GitHub Actions OIDC token using the provided ID request token and URL.
func (e *CognitoGHATokenExchanger) fetchGHAJWT() error {
	if e.ghaJWT != "" {
		return nil
	}
	fmt.Printf("::add-mask::%s\n", e.gha.IDTokenRequestToken)
	url := fmt.Sprintf("%s&audience=%s", e.gha.IDTokenRequestURL, audience)

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 5

	req, err := retryablehttp.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("could not create request for GHA token %s: %w", url, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.gha.IDTokenRequestToken))
	res, err := retryClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making http request for GHA token %s: %w", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	var resBody []byte
	resBody, _ = io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("non-200 response %d from GHA token request %s: %s", res.StatusCode, url, string(resBody))
	}

	var result map[string]any
	err = json.Unmarshal(resBody, &result)
	if err != nil {
		return fmt.Errorf("could not unmarshal response body for GHA token %s: %w", url, err)
	}
	if token, ok := result["value"].(string); ok {
		e.ghaJWT = token
		return nil
	}

	return fmt.Errorf("could not find token in response body for GHA token %s: %s", url, string(resBody))
}

// fetchCognitoOIDCToken retrieves an OIDC token from Cognito in exchange for a GitHub Actions JWT token.
func (e *CognitoGHATokenExchanger) fetchCognitoOIDCToken() error {
	if e.cognitoOIDCToken != "" {
		return nil
	}

	if e.ghaJWT == "" {
		if err := e.fetchGHAJWT(); err != nil {
			return fmt.Errorf("error fetching GHA JWT token: %w", err)
		}
	}

	if e.cognito.Region == "" {
		parts := strings.Split(e.cognito.IdentityPoolID, ":")
		e.cognito.Region = parts[0]
	}

	cognitoClient := cognitoidentity.New(cognitoidentity.Options{Region: e.cognito.Region})
	getIdOutput, err := cognitoClient.GetId(
		e.ctx,
		&cognitoidentity.GetIdInput{
			AccountId:      &e.cognito.AccountID,
			IdentityPoolId: &e.cognito.IdentityPoolID,
			Logins: map[string]string{
				loginProvider: e.ghaJWT,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("error invoking cognito-identity.getid(): %w", err)
	}

	getOpenIdTokenOutput, err := cognitoClient.GetOpenIdToken(
		e.ctx,
		&cognitoidentity.GetOpenIdTokenInput{
			IdentityId: getIdOutput.IdentityId,
			Logins: map[string]string{
				loginProvider: e.ghaJWT,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("error invoking cognito-identity.getopenidtoken(): %w", err)
	}

	e.cognitoOIDCToken = *getOpenIdTokenOutput.Token

	err = parseClaims("Cognito", e.cognitoOIDCToken, nil)
	if err != nil {
		return fmt.Errorf("error parsing Cognito OIDC token claims: %w", err)
	}

	return nil
}

// parseClaims outputs a list of claims from the provided JWT.
// accepts an optional map pointer to return specific claims by key.
func parseClaims(label, token string, returnClaims *map[string]any) error {
	claims := jwt.MapClaims{}
	// Signature will be verified by Cognito or STS, we can skip verification
	_, _, err := jwt.NewParser(jwt.WithPaddingAllowed()).ParseUnverified(token, claims)
	if err != nil {
		return fmt.Errorf("failed to parse unverified token: %w", err)
	}

	// Sort claims by key for consistent display
	keys := slices.Sorted(maps.Keys(claims))

	fmt.Printf("::group::Show %s JWT Claims\n-----------------\n", label)
	// replace unix timestamps with dates and extract values to return
	for _, key := range keys {
		value := claims[key]
		if key == "iat" || key == "exp" || key == "nbf" {
			// convert numeric date claims to human-readable format
			if floatVal, ok := value.(float64); ok {
				timeVal := time.Unix(int64(floatVal), 0)
				claims[key] = timeVal.Format(time.RFC3339)
			}
		}
		if returnClaims != nil {
			_, ok := (*returnClaims)[key]
			if ok {
				(*returnClaims)[key] = value
			}
		}
	}
	prettyJSON, err := json.MarshalIndent(claims, "  ", "    ")
	if err == nil {
		fmt.Println(string(prettyJSON))
	} else {
		for _, key := range keys {
			fmt.Printf("%s: %v\n", key, claims[key])
		}
	}
	fmt.Print("::endgroup::\n")

	return nil
}
