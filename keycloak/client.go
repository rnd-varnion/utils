package keycloak

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const SuperAdminSlug = "super_admin"

// AccountConfig is the Keycloak connection configuration shared by every
// service that imports this module. Each service maps its own config struct
// (loaded from YAML/env) into this type at the wiring point (main.go), so the
// shared package never depends on a service's config package.
type AccountConfig struct {
	RealmURL      string
	ClientID      string
	ClientSecret  string
	AdminURL      string
	AdminUsername string
	AdminPassword string
	AdminClientID string
	TargetRealm   string
}

// IsBusinessRole reports whether a client role is a business role assignable to
// a user, as opposed to a permission role. Permission roles use the
// "module.action" naming convention (e.g. "users.read"), while business roles
// are plain slugs without a dot (e.g. "admin", "technician", "testing_role").
func IsBusinessRole(name string) bool {
	return name != "" && !strings.Contains(name, ".")
}

type Client struct {
	conf       AccountConfig
	httpClient *http.Client
	mu         sync.RWMutex
	jwks       map[string]*rsa.PublicKey
	jwksUntil  time.Time
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// TokenError carries the Keycloak HTTP status and response body for a failed
// token-endpoint call. Callers use Transient to decide whether to retry.
type TokenError struct {
	StatusCode int
	Body       string
}

func (e *TokenError) Error() string {
	return fmt.Sprintf("keycloak token failed: %d %s", e.StatusCode, e.Body)
}

// Transient reports whether the failure is likely temporary (server error,
// rate limiting, or a network-level failure with no response) and worth
// retrying. Permanent failures (4xx) indicate an invalid/expired/revoked
// token and should not be retried.
func (e *TokenError) Transient() bool {
	return e.StatusCode == 0 || e.StatusCode >= 500 || e.StatusCode == 429
}

type Claims struct {
	Subject     string
	Email       string
	Name        string
	RoleSlug    string
	Permissions []string
	Raw         jwt.MapClaims
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func New(conf AccountConfig) *Client {
	return &Client{
		conf:       conf,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		jwks:       map[string]*rsa.PublicKey{},
	}
}

func ValidateAdmin(conf AccountConfig) error {
	if conf.RealmURL == "" || conf.ClientID == "" || conf.ClientSecret == "" {
		return errors.New("keycloak realm url, client id, and client secret are required")
	}
	if conf.AdminURL == "" || conf.AdminUsername == "" || conf.AdminPassword == "" || conf.AdminClientID == "" || conf.TargetRealm == "" {
		return errors.New("keycloak admin url, username, password, client id, and target realm are required")
	}
	return nil
}

func (c *Client) PasswordGrant(ctx context.Context, email, password string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", c.conf.ClientID)
	form.Set("client_secret", c.conf.ClientSecret)
	form.Set("username", email)
	form.Set("password", password)
	return c.token(ctx, form)
}

func (c *Client) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", c.conf.ClientID)
	form.Set("client_secret", c.conf.ClientSecret)
	form.Set("refresh_token", refreshToken)
	return c.token(ctx, form)
}

func (c *Client) Logout(ctx context.Context, refreshToken string) error {
	form := url.Values{}
	form.Set("client_id", c.conf.ClientID)
	form.Set("client_secret", c.conf.ClientSecret)
	form.Set("refresh_token", refreshToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.realmEndpoint("protocol/openid-connect/logout"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("keycloak logout failed: %s", resp.Status)
	}
	return nil
}

func (c *Client) VerifyAccessToken(ctx context.Context, tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing kid")
		}
		return c.publicKey(ctx, kid)
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	if !c.validClient(claims) {
		return nil, fmt.Errorf("invalid token client: configured client_id=%q not found in token claims (azp/aud/resource_access)", c.conf.ClientID)
	}
	return c.extractClaims(claims), nil
}

func (c *Client) ParseUnverified(tokenString string) *Claims {
	parser := jwt.NewParser()
	claims := jwt.MapClaims{}
	_, _, _ = parser.ParseUnverified(tokenString, claims)
	return c.extractClaims(claims)
}

func (c *Client) token(ctx context.Context, form url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.realmEndpoint("protocol/openid-connect/token"), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network/timeout error — no HTTP response received. Treat as transient.
		return nil, &TokenError{StatusCode: 0, Body: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, &TokenError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	var out TokenResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key := c.jwks[kid]
	fresh := time.Now().Before(c.jwksUntil)
	c.mu.RUnlock()
	if key != nil && fresh {
		return key, nil
	}
	if err := c.refreshJWKS(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	key = c.jwks[kid]
	if key == nil {
		return nil, errors.New("unknown kid")
	}
	return key, nil
}

func (c *Client) refreshJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.realmEndpoint("protocol/openid-connect/certs"), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("keycloak jwks failed: %s", resp.Status)
	}
	var data jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, jwk := range data.Keys {
		key, err := rsaKey(jwk)
		if err == nil && key != nil {
			keys[jwk.Kid] = key
		}
	}
	c.mu.Lock()
	c.jwks = keys
	c.jwksUntil = time.Now().Add(15 * time.Minute)
	c.mu.Unlock()
	return nil
}

func (c *Client) extractClaims(raw jwt.MapClaims) *Claims {
	realmRoleSlug := c.realmBusinessRole(raw)
	clientRoleSlug := c.clientBusinessRole(raw)
	permissions := c.clientPermissionRoles(raw)

	roleSlug := "user"
	if realmRoleSlug == SuperAdminSlug || clientRoleSlug == SuperAdminSlug {
		roleSlug = SuperAdminSlug
	} else if realmRoleSlug != "" {
		roleSlug = realmRoleSlug
	} else if clientRoleSlug != "" {
		roleSlug = clientRoleSlug
	}
	return &Claims{
		Subject:     stringClaim(raw, "sub"),
		Email:       firstString(raw, "email", "preferred_username"),
		Name:        firstString(raw, "name", "preferred_username", "email"),
		RoleSlug:    roleSlug,
		Permissions: permissions,
		Raw:         raw,
	}
}

// realmBusinessRole returns the business role carried in realm_access.roles,
// or "" when none is present. In the realm-role model, business roles live
// here so they are visible to every service regardless of which client issued
// the token.
func (c *Client) realmBusinessRole(raw jwt.MapClaims) string {
	realmAccess, _ := raw["realm_access"].(map[string]interface{})
	items, _ := realmAccess["roles"].([]interface{})
	var found string
	for _, item := range items {
		role, ok := item.(string)
		if !ok || !IsBusinessRole(role) {
			continue
		}
		if role == SuperAdminSlug {
			return role
		}
		if found == "" {
			found = role
		}
	}
	return found
}

// clientBusinessRole returns the business role carried in
// resource_access[client].roles (the legacy client-role model), or "". Kept as
// a fallback so tokens issued before the realm-role migration still resolve a
// role slug.
func (c *Client) clientBusinessRole(raw jwt.MapClaims) string {
	roles := c.clientRoles(raw)
	var found string
	for _, role := range roles {
		if !IsBusinessRole(role) {
			continue
		}
		if role == SuperAdminSlug {
			return role
		}
		if found == "" {
			found = role
		}
	}
	return found
}

// clientPermissionRoles returns the permission slugs (module.action) carried
// in resource_access[client].roles. Permissions stay client-scoped even after
// business roles move to realm level.
func (c *Client) clientPermissionRoles(raw jwt.MapClaims) []string {
	roles := c.clientRoles(raw)
	permissions := make([]string, 0, len(roles))
	for _, role := range roles {
		if IsBusinessRole(role) {
			continue
		}
		permissions = append(permissions, role)
	}
	return permissions
}

func (c *Client) clientRoles(raw jwt.MapClaims) []string {
	resourceAccess, _ := raw["resource_access"].(map[string]interface{})
	clientAccess, _ := resourceAccess[c.conf.ClientID].(map[string]interface{})
	items, _ := clientAccess["roles"].([]interface{})
	roles := make([]string, 0, len(items))
	for _, item := range items {
		if role, ok := item.(string); ok && role != "" {
			roles = append(roles, role)
		}
	}
	return roles
}

func (c *Client) validClient(raw jwt.MapClaims) bool {
	clientID := c.conf.ClientID
	if clientID == "" {
		return false
	}
	if stringClaim(raw, "azp") == clientID {
		return true
	}
	if hasAudience(raw["aud"], clientID) {
		return true
	}
	resourceAccess, _ := raw["resource_access"].(map[string]interface{})
	_, ok := resourceAccess[clientID]
	return ok
}

func hasAudience(raw interface{}, clientID string) bool {
	switch aud := raw.(type) {
	case string:
		return aud == clientID
	case []interface{}:
		for _, item := range aud {
			if value, ok := item.(string); ok && value == clientID {
				return true
			}
		}
	case []string:
		for _, value := range aud {
			if value == clientID {
				return true
			}
		}
	}
	return false
}

func (c *Client) realmEndpoint(path string) string {
	return strings.TrimRight(c.conf.RealmURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func rsaKey(jwk jwkKey) (*rsa.PublicKey, error) {
	if jwk.Kty != "RSA" || jwk.N == "" || jwk.E == "" {
		return nil, nil
	}
	n, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: e}, nil
}

func stringClaim(raw jwt.MapClaims, key string) string {
	value, _ := raw[key].(string)
	return value
}

func firstString(raw jwt.MapClaims, keys ...string) string {
	for _, key := range keys {
		if value := stringClaim(raw, key); value != "" {
			return value
		}
	}
	return ""
}

func JSONBody(v interface{}) (io.Reader, error) {
	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		return nil, err
	}
	return buf, nil
}
