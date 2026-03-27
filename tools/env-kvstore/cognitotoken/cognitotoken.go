package cognitotoken

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/big"
	"net/http"
	"slices"
	"time"

	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"
	"github.com/gravitational/shared-workflows/tools/env-kvstore/actions"

	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentity"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/golang-jwt/jwt/v5"
	"github.com/hashicorp/go-retryablehttp"
)

const (
	audience         = "cognito-identity.amazonaws.com"
	ghaJWKSURL       = "https://token.actions.githubusercontent.com/.well-known/jwks"
	fmtIssuer        = "https://token.actions.githubusercontent.com%s"
	fmtLoginProvider = "token.actions.githubusercontent.com%s"

	minSessionDuration = 15 * time.Minute // matches Cognito's default

	githubStepName = "Exchange GHA token for AWS credentials"
)

// CognitoGHATokenExchanger creates a role provider that exchanges a GitHub Actions JWT token for a Cognito OIDC token.
// The Cognito OIDC token can include claims that map to role session tags for IAM ABAC. Implements stscreds.IdentityTokenRetriever.
type CognitoGHATokenExchanger struct {
	Claims           config.GHAClaims
	ghaJWT           string
	cognitoOIDCToken string
	ctx              context.Context

	// skipValidation is used for testing only
	skipValidation bool

	gha     config.GHAConfig
	cognito config.CognitoConfig
}

type jwk struct {
	KTY string `json:"kty"`
	KID string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

// NewTokenExchanger creates a new CognitoGHATokenExchanger with the provided Cognito and GHA configuration.
func NewTokenExchanger(ctx context.Context, cognitoConfig *config.CognitoConfig, ghaConfig *config.GHAConfig) *CognitoGHATokenExchanger {
	return &CognitoGHATokenExchanger{
		ctx:     ctx,
		ghaJWT:  "",
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
		actions.AddSummary(githubStepName, actions.SummaryRow{
			Result: actions.SummaryResultFailure,
			Msg:    fmt.Sprintf("Failed to complete token exchange: %v", err),
		})
		return nil, fmt.Errorf("error getting AWS session name for Cognito role provider: %w", err)
	}

	provider := stscreds.NewWebIdentityRoleProvider(
		sts.New(sts.Options{Region: e.cognito.GetRegion()}),
		e.cognito.RoleARN,
		e,
		func(opt *stscreds.WebIdentityRoleOptions) {
			opt.RoleSessionName = sessionName
			opt.Duration = minSessionDuration
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

	if err := logClaims("GHA", e.ghaJWT); err != nil {
		return "", fmt.Errorf("error logging GHA JWT claims: %w", err)
	}

	// token signature was already validated in fetchGHAJWT, so skipping validation here
	token, _, err := jwt.NewParser(jwt.WithPaddingAllowed()).ParseUnverified(e.ghaJWT, &config.GHAClaims{})
	if err != nil {
		return "", fmt.Errorf("error parsing claims to GHAClaims struct: %w", err)
	}
	c, ok := token.Claims.(*config.GHAClaims)
	if !ok || c == nil {
		return "", fmt.Errorf("error asserting GHA claims to GHAClaims struct")
	}
	e.Claims = *c

	if e.Claims.RunID == "" || e.Claims.SHA == "" {
		return "", fmt.Errorf("missing required claim values in GHA JWT token: run_id and sha are required: run_id=%s, sha=%s", e.Claims.RunID, e.Claims.SHA)
	}

	sessionName := fmt.Sprintf("%s@%s", e.Claims.RunID, e.Claims.SHA)
	slog.Info("Using session name.", "sessionName", sessionName)
	return sessionName, nil
}

// fetchGHAJWT retrieves a GitHub Actions OIDC token using the provided ID request token and URL.
func (e *CognitoGHATokenExchanger) fetchGHAJWT() error {
	if e.ghaJWT != "" {
		return nil
	}
	fmt.Printf("\n::add-mask::%s\n", e.gha.IDTokenRequestToken)
	url := fmt.Sprintf("%s&audience=%s", e.gha.IDTokenRequestURL, audience)

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.HTTPClient.Timeout = 10 * time.Second

	req, err := retryablehttp.NewRequestWithContext(e.ctx, http.MethodGet, url, nil)
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
		if !e.skipValidation {
			if err := e.validateGHAToken(); err != nil {
				e.ghaJWT = ""
				return fmt.Errorf("failed to validate GHA token: %w", err)
			}
		}
		actions.AddSummary(githubStepName, actions.SummaryRow{
			Result: actions.SummaryResultSuccess,
			Msg:    "Retrieved GHA JWT from ID Token request URL",
		})
		return nil
	}

	return fmt.Errorf("could not find token in response body for GHA token %s: %s", url, string(resBody))
}

func (e *CognitoGHATokenExchanger) validateGHAToken() error {
	if e.ghaJWT == "" {
		return fmt.Errorf("cannot validate empty GHA JWT")
	}
	token, err := jwt.Parse(
		e.ghaJWT,
		func(token *jwt.Token) (any, error) {
			kid, ok := token.Header["kid"].(string)
			if !ok {
				return nil, fmt.Errorf("token header does not contain kid")
			}

			publicKey, err := e.getGHAPublicKey(kid)
			if err != nil {
				return nil, fmt.Errorf("failed to retrieve GHA public key: %w", err)
			}

			return publicKey, nil
		},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
	)

	if err != nil {
		return fmt.Errorf("error validating token: %w", err)
	}
	if !token.Valid {
		return errors.New("token is invalid")
	}
	tokenIssuer, err := token.Claims.GetIssuer()
	if err != nil {
		return fmt.Errorf("token missing expected issuer claim: %w", err)
	}

	var issuer string
	if e.gha.EnterpriseName != "" {
		issuer = fmt.Sprintf(fmtIssuer, fmt.Sprintf("/%s", e.gha.EnterpriseName))
	} else {
		issuer = fmt.Sprintf(fmtIssuer, "")
	}
	if tokenIssuer != issuer {
		return fmt.Errorf("unexpected token issuer, got: %s, expected: %s", tokenIssuer, issuer)
	}

	tokenAudience, err := token.Claims.GetAudience()
	if err != nil {
		return fmt.Errorf("token missing expected audience claim: %w", err)
	}
	if !slices.Contains(tokenAudience, audience) {
		return fmt.Errorf("unexpected token audience: %v", tokenAudience)
	}
	return nil
}

func (e *CognitoGHATokenExchanger) getGHAPublicKey(kid string) (*rsa.PublicKey, error) {
	req, err := retryablehttp.NewRequestWithContext(e.ctx, http.MethodGet, ghaJWKSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating JWKS request: %w", err)
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.HTTPClient.Timeout = 5 * time.Second
	resp, err := retryClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed fetching JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("JWKS endpoint non-200 return status %d: %s", resp.StatusCode, body)
	}

	var ghaJWKS jwks
	if err := json.NewDecoder(resp.Body).Decode(&ghaJWKS); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWKS: %w", err)
	}

	for _, key := range ghaJWKS.Keys {
		if key.KID == kid {
			if key.KTY != "RSA" {
				return nil, fmt.Errorf("expected keytype RSA, got %s", key.KTY)
			}

			nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
			if err != nil {
				return nil, fmt.Errorf("failed to decode modulus: %w", err)
			}
			n := new(big.Int).SetBytes(nBytes)
			eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
			if err != nil {
				return nil, fmt.Errorf("failed to decode exponent: %w", err)
			}
			e := new(big.Int).SetBytes(eBytes)

			return &rsa.PublicKey{
				N: n,
				E: int(e.Int64()),
			}, nil
		}
	}
	return nil, fmt.Errorf("could not find public key with ID %s", kid)
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

	var loginProvider string
	if e.gha.EnterpriseName != "" {
		loginProvider = fmt.Sprintf(fmtLoginProvider, fmt.Sprintf("/%s", e.gha.EnterpriseName))
	} else {
		loginProvider = fmt.Sprintf(fmtLoginProvider, "")
	}

	cognitoClient := cognitoidentity.New(cognitoidentity.Options{Region: e.cognito.GetRegion()})
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

	if err = logClaims("Cognito", e.cognitoOIDCToken); err != nil {
		return fmt.Errorf("error logging Cognito OIDC token claims: %w", err)
	}

	actions.AddSummary(githubStepName, actions.SummaryRow{
		Result: actions.SummaryResultSuccess,
		Msg:    "Retrieved OIDC token from Cognito",
	})
	return nil
}

// logClaims outputs a list of claims from the provided JWT.
func logClaims(label, token string) error {
	mapClaims := jwt.MapClaims{}
	// Skipping validation since this is only for logging purposes
	_, _, err := jwt.NewParser(jwt.WithPaddingAllowed()).ParseUnverified(token, mapClaims)
	if err != nil {
		return fmt.Errorf("failed to parse unverified token: %w", err)
	}

	// Sort claims by key for consistent display
	keys := slices.Sorted(maps.Keys(mapClaims))

	fmt.Printf("\n::group::Show %s JWT Claims\n", label)
	defer fmt.Println("\n::endgroup::")
	// replace unix timestamps with dates and extract values to return
	for _, key := range keys {
		value := mapClaims[key]
		if key == "iat" || key == "exp" || key == "nbf" {
			// convert numeric date claims to human-readable format
			if floatVal, ok := value.(float64); ok {
				timeVal := time.Unix(int64(floatVal), 0)
				mapClaims[key] = timeVal.Format(time.RFC3339)
			}
		}
	}
	prettyJSON, err := json.MarshalIndent(mapClaims, "  ", "    ")
	if err == nil {
		fmt.Println(string(prettyJSON))
	} else {
		for _, key := range keys {
			fmt.Printf("%s: %v\n", key, mapClaims[key])
		}
	}
	return nil
}
