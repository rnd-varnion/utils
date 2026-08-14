package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type Role struct {
	ID          string              `json:"id,omitempty"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Composite   bool                `json:"composite,omitempty"`
	Attributes  map[string][]string `json:"attributes,omitempty"`
}

type User struct {
	ID               string              `json:"id,omitempty"`
	Username         string              `json:"username"`
	Email            string              `json:"email"`
	FirstName        string              `json:"firstName,omitempty"`
	LastName         string              `json:"lastName,omitempty"`
	Enabled          bool                `json:"enabled"`
	EmailVerified    bool                `json:"emailVerified"`
	RequiredActions  []string            `json:"requiredActions"`
	Attributes       map[string][]string `json:"attributes,omitempty"`
	CreatedTimestamp int64               `json:"createdTimestamp,omitempty"`
}

type Credential struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

type Realm struct {
	Realm   string `json:"realm"`
	Enabled bool   `json:"enabled"`
}

type ClientRepresentation struct {
	ClientID                  string   `json:"clientId"`
	Name                      string   `json:"name,omitempty"`
	Enabled                   bool     `json:"enabled"`
	Protocol                  string   `json:"protocol"`
	PublicClient              bool     `json:"publicClient"`
	Secret                    string   `json:"secret,omitempty"`
	DirectAccessGrantsEnabled bool     `json:"directAccessGrantsEnabled"`
	ServiceAccountsEnabled    bool     `json:"serviceAccountsEnabled"`
	StandardFlowEnabled       bool     `json:"standardFlowEnabled"`
	RedirectURIs              []string `json:"redirectUris,omitempty"`
	WebOrigins                []string `json:"webOrigins,omitempty"`
}

func (c *Client) Bootstrap(ctx context.Context) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return err
	}
	if err := c.ensureRealm(ctx, token); err != nil {
		return err
	}
	return c.ensureClient(ctx, token)
}

func (c *Client) ensureRealm(ctx context.Context, token string) error {
	realm := c.targetRealm()
	if realm == "" {
		return fmt.Errorf("keycloak target realm is required")
	}
	if _, err := c.adminRequest(ctx, http.MethodGet, c.adminRealmEndpoint("realms/%s", realm), token, nil); err == nil {
		return nil
	}
	body, _ := JSONBody(Realm{Realm: realm, Enabled: true})
	_, err := c.adminRequest(ctx, http.MethodPost, c.adminRealmEndpoint("realms"), token, body)
	return err
}

func (c *Client) ensureClient(ctx context.Context, token string) error {
	if c.conf.ClientID == "" {
		return fmt.Errorf("keycloak client id is required")
	}
	rep := ClientRepresentation{
		ClientID:                  c.conf.ClientID,
		Name:                      c.conf.ClientID,
		Enabled:                   true,
		Protocol:                  "openid-connect",
		PublicClient:              false,
		Secret:                    c.conf.ClientSecret,
		DirectAccessGrantsEnabled: true,
		ServiceAccountsEnabled:    true,
		StandardFlowEnabled:       true,
		RedirectURIs:              []string{"*"},
		WebOrigins:                []string{"*"},
	}
	if _, err := c.clientUUID(ctx, token); err == nil {
		return nil
	}
	body, _ := JSONBody(rep)
	_, err := c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("clients"), token, body)
	return err
}

func (c *Client) UpsertClientRole(ctx context.Context, name, description string, attributes map[string][]string) error {
	clientID, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	roleURL := c.adminEndpoint("clients/%s/roles/%s", clientID, url.PathEscape(name))
	if _, err := c.adminRequest(ctx, http.MethodGet, roleURL, token, nil); err == nil {
		body, _ := JSONBody(Role{Name: name, Description: description, Attributes: attributes})
		_, err = c.adminRequest(ctx, http.MethodPut, roleURL, token, body)
		return err
	}
	body, _ := JSONBody(Role{Name: name, Description: description, Attributes: attributes})
	_, err = c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("clients/%s/roles", clientID), token, body)
	if err != nil && strings.Contains(err.Error(), "409") {
		return nil
	}
	return err
}

func (c *Client) UpsertClientRoleForClient(ctx context.Context, clientID, name, description string, attributes map[string][]string) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return err
	}
	uid, err := c.clientUUIDByName(ctx, token, clientID)
	if err != nil {
		return err
	}
	roleURL := c.adminEndpoint("clients/%s/roles/%s", uid, url.PathEscape(name))
	if _, err := c.adminRequest(ctx, http.MethodGet, roleURL, token, nil); err == nil {
		body, _ := JSONBody(Role{Name: name, Description: description, Attributes: attributes})
		_, err = c.adminRequest(ctx, http.MethodPut, roleURL, token, body)
		return err
	}
	body, _ := JSONBody(Role{Name: name, Description: description, Attributes: attributes})
	_, err = c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("clients/%s/roles", uid), token, body)
	if err != nil && strings.Contains(err.Error(), "409") {
		return nil
	}
	return err
}

func (c *Client) DeleteClientRole(ctx context.Context, name string) error {
	clientID, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	_, err = c.adminRequest(ctx, http.MethodDelete, c.adminEndpoint("clients/%s/roles/%s", clientID, url.PathEscape(name)), token, nil)
	return err
}

func (c *Client) ListClientRolesPublic(ctx context.Context) ([]Role, error) {
	clientID, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("clients/%s/roles?briefRepresentation=false", clientID), token, nil)
	if err != nil {
		return nil, err
	}
	var roles []Role
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

func (c *Client) GetClientRolePublic(ctx context.Context, name string) (*Role, error) {
	clientID, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	return c.getClientRole(ctx, clientID, token, name)
}

func (c *Client) GetRoleComposites(ctx context.Context, roleName string) ([]Role, error) {
	clientID, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("clients/%s/roles/%s/composites", clientID, url.PathEscape(roleName)), token, nil)
	if err != nil {
		return nil, err
	}
	var roles []Role
	_ = json.Unmarshal(body, &roles)
	return roles, nil
}

func (c *Client) SyncCompositeRoles(ctx context.Context, roleName string, permissionSlugs []string) error {
	clientID, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	role, err := c.getClientRole(ctx, clientID, token, roleName)
	if err != nil {
		return err
	}
	for _, slug := range permissionSlugs {
		if err := c.UpsertClientRole(ctx, slug, slug, nil); err != nil {
			return err
		}
	}
	currentURL := c.adminEndpoint("clients/%s/roles/%s/composites/clients/%s", clientID, url.PathEscape(roleName), clientID)
	currentBody, err := c.adminRequest(ctx, http.MethodGet, currentURL, token, nil)
	if err != nil {
		return err
	}
	var current []Role
	_ = json.Unmarshal(currentBody, &current)
	if len(current) > 0 {
		body, _ := JSONBody(current)
		if _, err := c.adminRequest(ctx, http.MethodDelete, c.adminEndpoint("roles-by-id/%s/composites", role.ID), token, body); err != nil {
			return err
		}
	}
	if len(permissionSlugs) == 0 {
		return nil
	}
	roles := make([]Role, 0, len(permissionSlugs))
	for _, slug := range permissionSlugs {
		perm, err := c.getClientRole(ctx, clientID, token, slug)
		if err != nil {
			return err
		}
		roles = append(roles, *perm)
	}
	body, _ := JSONBody(roles)
	_, err = c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("roles-by-id/%s/composites", role.ID), token, body)
	return err
}

func (c *Client) SyncCompositeRolesMulti(ctx context.Context, roleName string, slugToClient map[string]string) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return err
	}
	// Business role lives on the configured client.
	clientID, err := c.clientUUID(ctx, token)
	if err != nil {
		return err
	}
	role, err := c.getClientRole(ctx, clientID, token, roleName)
	if err != nil {
		return err
	}

	// Ensure each permission role exists on its client and collect its Role rep.
	// A composite child can reference a role on any client (cross-client composite).
	roles := make([]Role, 0, len(slugToClient))
	for slug, targetClient := range slugToClient {
		if err := c.UpsertClientRoleForClient(ctx, targetClient, slug, slug, nil); err != nil {
			return err
		}
		uid, err := c.clientUUIDByName(ctx, token, targetClient)
		if err != nil {
			return err
		}
		perm, err := c.getClientRole(ctx, uid, token, slug)
		if err != nil {
			return err
		}
		roles = append(roles, *perm)
	}

	// Fetch ALL current composites (unfiltered → includes other clients) and delete.
	currentBody, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("clients/%s/roles/%s/composites", clientID, url.PathEscape(roleName)), token, nil)
	if err != nil {
		return err
	}
	var current []Role
	_ = json.Unmarshal(currentBody, &current)
	if len(current) > 0 {
		body, _ := JSONBody(current)
		if _, err := c.adminRequest(ctx, http.MethodDelete, c.adminEndpoint("roles-by-id/%s/composites", role.ID), token, body); err != nil {
			return err
		}
	}
	if len(roles) == 0 {
		return nil
	}
	body, _ := JSONBody(roles)
	_, err = c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("roles-by-id/%s/composites", role.ID), token, body)
	return err
}

func (c *Client) UpsertUser(ctx context.Context, email, name, password, roleName string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	user, err := c.findUser(ctx, token, email)
	if err != nil {
		return err
	}
	userID := ""
	if user != nil {
		userID = user.ID
	}
	firstName, lastName := splitName(name)
	if userID == "" {
		body, _ := JSONBody(User{Username: email, Email: email, FirstName: firstName, LastName: lastName, Enabled: true, EmailVerified: true, RequiredActions: []string{}})
		if _, err := c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("users"), token, body); err != nil {
			return err
		}
		userID, err = c.findUserID(ctx, token, email)
		if err != nil {
			return err
		}
	} else {
		user.Username = email
		user.Email = email
		user.FirstName = firstName
		user.LastName = lastName
		user.EmailVerified = true
		user.RequiredActions = []string{}
		body, _ := JSONBody(user)
		if _, err := c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s", userID), token, body); err != nil {
			return err
		}
	}
	if userID == "" {
		return fmt.Errorf("keycloak user not found after create")
	}
	if password != "" {
		body, _ := JSONBody(Credential{Type: "password", Value: password, Temporary: false})
		if _, err := c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s/reset-password", userID), token, body); err != nil {
			return err
		}
	}
	return c.setUserRealmRole(ctx, token, userID, roleName)
}

func (c *Client) EnsureSeedUser(ctx context.Context, email, name, password, roleName string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	user, err := c.findUser(ctx, token, email)
	if err != nil {
		return err
	}
	if user != nil {
		return c.setUserRealmRole(ctx, token, user.ID, roleName)
	}
	firstName, lastName := splitName(name)
	body, _ := JSONBody(User{Username: email, Email: email, FirstName: firstName, LastName: lastName, Enabled: true, EmailVerified: true, RequiredActions: []string{}})
	if _, err := c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("users"), token, body); err != nil {
		return err
	}
	userID, err := c.findUserID(ctx, token, email)
	if err != nil {
		return err
	}
	if password != "" {
		body, _ := JSONBody(Credential{Type: "password", Value: password, Temporary: false})
		if _, err := c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s/reset-password", userID), token, body); err != nil {
			return err
		}
	}
	return c.setUserRealmRole(ctx, token, userID, roleName)
}

func (c *Client) EnsureSeedUserWithAttrs(ctx context.Context, email, name, password, roleName string, attributes map[string][]string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	user, err := c.findUser(ctx, token, email)
	if err != nil {
		return err
	}
	if user != nil {
		return c.setUserRealmRole(ctx, token, user.ID, roleName)
	}
	firstName, lastName := splitName(name)
	body, _ := JSONBody(User{
		Username:      email,
		Email:         email,
		FirstName:     firstName,
		LastName:      lastName,
		Enabled:       true,
		EmailVerified: true,
		Attributes:    attributes,
	})
	if _, err := c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("users"), token, body); err != nil {
		return err
	}
	userID, err := c.findUserID(ctx, token, email)
	if err != nil {
		return err
	}
	if password != "" {
		body, _ := JSONBody(Credential{Type: "password", Value: password, Temporary: false})
		if _, err := c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s/reset-password", userID), token, body); err != nil {
			return err
		}
	}
	return c.setUserRealmRole(ctx, token, userID, roleName)
}

func (c *Client) CreateUser(ctx context.Context, email, name, password, roleName string, attributes map[string][]string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	existing, err := c.findUser(ctx, token, email)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("user already exists: %s", email)
	}
	firstName, lastName := splitName(name)
	user := User{
		Username:      email,
		Email:         email,
		FirstName:     firstName,
		LastName:      lastName,
		Enabled:       true,
		EmailVerified: true,
		Attributes:    attributes,
	}
	body, _ := JSONBody(user)
	if _, err := c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("users"), token, body); err != nil {
		return err
	}
	userID, err := c.findUserID(ctx, token, email)
	if err != nil {
		return err
	}
	if userID == "" {
		return fmt.Errorf("keycloak user not found after create")
	}
	if password != "" {
		body, _ := JSONBody(Credential{Type: "password", Value: password, Temporary: false})
		if _, err := c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s/reset-password", userID), token, body); err != nil {
			return err
		}
	}
	return c.setUserRealmRole(ctx, token, userID, roleName)
}

func (c *Client) SendPasswordResetEmail(ctx context.Context, email string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	userID, err := c.findUserID(ctx, token, email)
	if err != nil {
		return err
	}
	if userID == "" {
		return nil
	}
	body, _ := JSONBody([]string{"UPDATE_PASSWORD"})
	_, err = c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s/execute-actions-email", userID), token, body)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) ResetPasswordByEmail(ctx context.Context, email, password string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	userID, err := c.findUserID(ctx, token, email)
	if err != nil {
		return err
	}
	if userID == "" {
		return fmt.Errorf("keycloak user not found")
	}
	body, _ := JSONBody(Credential{Type: "password", Value: password, Temporary: false})
	_, err = c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s/reset-password", userID), token, body)
	return err
}

func (c *Client) SetUserEnabledByEmail(ctx context.Context, email string, enabled bool) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	userID, err := c.findUserID(ctx, token, email)
	if err != nil {
		return err
	}
	if userID == "" {
		return fmt.Errorf("keycloak user not found")
	}
	user, err := c.findUser(ctx, token, email)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("keycloak user not found")
	}
	user.Enabled = enabled
	body, _ := JSONBody(user)
	_, err = c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s", userID), token, body)
	return err
}

func (c *Client) UpdateUserProfileByEmail(ctx context.Context, oldEmail, newEmail, name string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	userID, err := c.findUserID(ctx, token, oldEmail)
	if err != nil {
		return err
	}
	if userID == "" {
		return fmt.Errorf("keycloak user not found")
	}
	user, err := c.findUser(ctx, token, oldEmail)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("keycloak user not found")
	}
	firstName, lastName := splitName(name)
	user.Username = newEmail
	user.Email = newEmail
	user.FirstName = firstName
	user.LastName = lastName
	user.EmailVerified = true
	body, _ := JSONBody(user)
	_, err = c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s", userID), token, body)
	return err
}

func (c *Client) UserExistsByEmail(ctx context.Context, email string) (bool, error) {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return false, err
	}
	user, err := c.findUser(ctx, token, email)
	if err != nil {
		return false, err
	}
	return user != nil, nil
}

func (c *Client) GetUserByEmailPublic(ctx context.Context, email string) (*User, error) {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	return c.findUser(ctx, token, email)
}

func (c *Client) GetUserAttributes(ctx context.Context, email string) (map[string][]string, error) {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	user, err := c.findUser(ctx, token, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("keycloak user not found")
	}
	return user.Attributes, nil
}

func (c *Client) ListUsers(ctx context.Context, search string, first, max int) ([]User, error) {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := c.adminEndpoint("users?first=%d&max=%d", first, max)
	if search != "" {
		endpoint = c.adminEndpoint("users?search=%s&first=%d&max=%d", url.QueryEscape(search), first, max)
	}
	body, err := c.adminRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *Client) ListUsersByAttribute(ctx context.Context, key, value string, max int) ([]User, error) {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	if max <= 0 {
		max = 500
	}
	endpoint := c.adminEndpoint("users?q=%s:%s&max=%d", url.QueryEscape(key), url.QueryEscape(value), max)
	body, err := c.adminRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *Client) ListAllUsers(ctx context.Context, maxResults int) ([]User, error) {
	if maxResults <= 0 {
		maxResults = 1000
	}
	var all []User
	first := 0
	pageSize := 100
	for first < maxResults {
		users, err := c.ListUsers(ctx, "", first, pageSize)
		if err != nil || len(users) == 0 {
			break
		}
		all = append(all, users...)
		if len(users) < pageSize {
			break
		}
		first += pageSize
	}
	return all, nil
}

func (c *Client) GetUserByID(ctx context.Context, userID string) (*User, error) {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("users/%s", userID), token, nil)
	if err != nil {
		return nil, err
	}
	var user User
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) GetUserClientRoles(ctx context.Context, userID string) ([]string, error) {
	roles, err := c.GetUserClientRolesFull(ctx, userID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(roles))
	for _, r := range roles {
		names = append(names, r.Name)
	}
	return names, nil
}

func (c *Client) GetUserClientRolesFull(ctx context.Context, userID string) ([]Role, error) {
	clientID, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("users/%s/role-mappings/clients/%s", userID, clientID), token, nil)
	if err != nil {
		return nil, err
	}
	var roles []Role
	_ = json.Unmarshal(body, &roles)
	return roles, nil
}

type UserProfileAttribute struct {
	Name        string              `json:"name"`
	DisplayName string              `json:"displayName,omitempty"`
	Permissions map[string][]string `json:"permissions,omitempty"`
}

func (c *Client) ConfigureUserProfile(ctx context.Context) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return err
	}

	// Step 1: enable declarative user profile for the realm.
	realmBody, _ := JSONBody(map[string]interface{}{
		"attributes": map[string]string{
			"userProfileEnabled": "true",
		},
	})
	if _, err := c.adminRequest(ctx, http.MethodPut, c.adminRealmEndpoint("realms/%s", c.targetRealm()), token, realmBody); err != nil {
		return fmt.Errorf("enable user profile: %w", err)
	}

	// Step 2: GET current profile config (to preserve default attributes).
	profileEndpoint := strings.TrimRight(c.conf.AdminURL, "/") + "/admin/realms/" + c.targetRealm() + "/users/profile"
	currentBody, err := c.adminRequest(ctx, http.MethodGet, profileEndpoint, token, nil)
	if err != nil {
		return fmt.Errorf("get user profile: %w", err)
	}

	// Step 3: merge custom attributes + set unmanaged policy = ENABLED.
	var raw map[string]interface{}
	if err := json.Unmarshal(currentBody, &raw); err != nil {
		return fmt.Errorf("parse user profile: %w", err)
	}

	existingAttrs, _ := raw["attributes"].([]interface{})
	attrNames := map[string]bool{}
	for _, a := range existingAttrs {
		if m, ok := a.(map[string]interface{}); ok {
			if n, _ := m["name"].(string); n != "" {
				attrNames[n] = true
			}
		}
	}

	customAttrs := []UserProfileAttribute{
		{Name: "phone_number", DisplayName: "Phone Number", Permissions: map[string][]string{"view": {"admin", "user"}, "edit": {"admin", "user"}}},
		{Name: "date_of_birth", DisplayName: "Date of Birth", Permissions: map[string][]string{"view": {"admin", "user"}, "edit": {"admin", "user"}}},
		{Name: "province_id", DisplayName: "Province", Permissions: map[string][]string{"view": {"admin", "user"}, "edit": {"admin", "user"}}},
		{Name: "city_id", DisplayName: "City", Permissions: map[string][]string{"view": {"admin", "user"}, "edit": {"admin", "user"}}},
		{Name: "address", DisplayName: "Address", Permissions: map[string][]string{"view": {"admin", "user"}, "edit": {"admin", "user"}}},
		{Name: "postal_code", DisplayName: "Postal Code", Permissions: map[string][]string{"view": {"admin", "user"}, "edit": {"admin", "user"}}},
		{Name: "latitude", DisplayName: "Latitude", Permissions: map[string][]string{"view": {"admin", "user"}, "edit": {"admin", "user"}}},
		{Name: "longitude", DisplayName: "Longitude", Permissions: map[string][]string{"view": {"admin", "user"}, "edit": {"admin", "user"}}},
	}
	for _, ca := range customAttrs {
		if !attrNames[ca.Name] {
			raw["attributes"] = append(raw["attributes"].([]interface{}), ca)
		}
	}
	raw["unmanagedAttributePolicy"] = "ENABLED"

	// Step 4: PUT merged config.
	mergedBody, _ := JSONBody(raw)
	_, err = c.adminRequest(ctx, http.MethodPut, profileEndpoint, token, mergedBody)
	if err != nil {
		return fmt.Errorf("configure user profile: %w", err)
	}
	return nil
}

// RealmTokenSettings controls the session/token lifespans applied to the
// Keycloak realm. All values are in seconds.
type RealmTokenSettings struct {
	AccessTokenLifespan int // access token validity (default 15 minutes)
	SSOIdleTimeout      int // refresh token idle timeout (default 60 minutes)
	SSOMaxLifespan      int // absolute session cap (default 7 days)
}

// DefaultRealmTokenSettings holds the recommended token lifespans for the
// prosky-admin realm. Used by the seeder so a fresh realm gets sensible
// values without manual Keycloak Admin UI configuration.
var DefaultRealmTokenSettings = RealmTokenSettings{
	AccessTokenLifespan: 15 * 60,       // 15 minutes
	SSOIdleTimeout:      60 * 60,       // 60 minutes
	SSOMaxLifespan:      7 * 24 * 3600, // 7 days
}

// ConfigureRealmTokens applies token/session lifespan settings to the target
// realm. It performs a partial PUT (Keycloak merges the fields), so only the
// token-related fields are touched — other realm settings are preserved.
func (c *Client) ConfigureRealmTokens(ctx context.Context, s RealmTokenSettings) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return err
	}
	body, _ := JSONBody(map[string]int{
		"accessTokenLifespan":   s.AccessTokenLifespan,
		"ssoSessionIdleTimeout": s.SSOIdleTimeout,
		"ssoSessionMaxLifespan": s.SSOMaxLifespan,
	})
	if _, err := c.adminRequest(ctx, http.MethodPut, c.adminRealmEndpoint("realms/%s", c.targetRealm()), token, body); err != nil {
		return fmt.Errorf("configure realm tokens: %w", err)
	}
	return nil
}

func (c *Client) CountUsers(ctx context.Context, search string) (int64, error) {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return 0, err
	}
	endpoint := c.adminEndpoint("users/count")
	if search != "" {
		endpoint = c.adminEndpoint("users/count?search=%s", url.QueryEscape(search))
	}
	body, err := c.adminRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return 0, err
	}
	var count int64
	_ = json.Unmarshal(body, &count)
	return count, nil
}

func (c *Client) DeleteUserByID(ctx context.Context, userID string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	_, err = c.adminRequest(ctx, http.MethodDelete, c.adminEndpoint("users/%s", userID), token, nil)
	return err
}

// RevokeUserSessions logs out all active sessions of a user (POST
// /users/{id}/logout), invalidating their refresh tokens. The already-issued
// access token remains valid until its natural expiry (stateless JWT), but the
// user must re-authenticate to refresh.
func (c *Client) RevokeUserSessions(ctx context.Context, userID string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	_, err = c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("users/%s/logout", url.PathEscape(userID)), token, nil)
	return err
}

// ListUsersByClientRole returns every user assigned to a client role, paging
// through the full result set. Used to enumerate affected users when a role's
// permissions change.
func (c *Client) ListUsersByClientRole(ctx context.Context, roleName string) ([]User, error) {
	clientID, token, err := c.adminContext(ctx)
	if err != nil {
		return nil, err
	}
	var all []User
	const pageSize = 100
	for first := 0; ; first += pageSize {
		endpoint := c.adminEndpoint("clients/%s/roles/%s/users?first=%d&max=%d",
			clientID, url.PathEscape(roleName), first, pageSize)
		body, err := c.adminRequest(ctx, http.MethodGet, endpoint, token, nil)
		if err != nil {
			return nil, err
		}
		var page []User
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
	}
	return all, nil
}

// RevokeRoleUserSessions revokes the active sessions of every user assigned to
// a client role, so they must re-authenticate and pick up the role's current
// permissions. It runs the per-user revokes concurrently (10 at a time).
// Returns (revoked, failed). Best-effort: a per-user failure is counted but
// does not stop the batch.
func (c *Client) RevokeRoleUserSessions(ctx context.Context, roleName string) (revoked, failed int, err error) {
	users, err := c.ListUsersByClientRole(ctx, roleName)
	if err != nil {
		return 0, 0, fmt.Errorf("list users by role %s: %w", roleName, err)
	}
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, u := range users {
		if u.ID == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(userID string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := c.RevokeUserSessions(ctx, userID); err != nil {
				mu.Lock()
				failed++
				mu.Unlock()
			} else {
				mu.Lock()
				revoked++
				mu.Unlock()
			}
		}(u.ID)
	}
	wg.Wait()
	return revoked, failed, nil
}

func (c *Client) AssignRoleToUser(ctx context.Context, userID, roleSlug string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	return c.setUserRealmRole(ctx, token, userID, roleSlug)
}

func (c *Client) UpdateUserByID(ctx context.Context, userID, name string, attributes map[string][]string) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	firstName, lastName := splitName(name)
	user := User{
		FirstName:  firstName,
		LastName:   lastName,
		Attributes: attributes,
	}
	body, _ := JSONBody(user)
	_, err = c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s", userID), token, body)
	return err
}

func (c *Client) UpdateUserFull(ctx context.Context, userID string, user User) error {
	_, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	body, _ := JSONBody(user)
	_, err = c.adminRequest(ctx, http.MethodPut, c.adminEndpoint("users/%s", userID), token, body)
	return err
}

func splitName(name string) (string, string) {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "User", "Seed"
	}
	if len(parts) == 1 {
		return parts[0], "Seed"
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func (c *Client) findUserID(ctx context.Context, token, email string) (string, error) {
	user, err := c.findUser(ctx, token, email)
	if err != nil || user == nil {
		return "", err
	}
	return user.ID, nil
}

func (c *Client) findUser(ctx context.Context, token, email string) (*User, error) {
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("users?email=%s&exact=true", url.QueryEscape(email)), token, nil)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}

func (c *Client) setUserClientRole(ctx context.Context, clientID, token, userID, roleName string) error {
	currentBody, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("users/%s/role-mappings/clients/%s", userID, clientID), token, nil)
	if err != nil {
		return err
	}
	var current []Role
	_ = json.Unmarshal(currentBody, &current)
	remove := make([]Role, 0, len(current))
	for _, role := range current {
		if IsBusinessRole(role.Name) {
			remove = append(remove, role)
		}
	}
	if len(remove) > 0 {
		body, _ := JSONBody(remove)
		if _, err := c.adminRequest(ctx, http.MethodDelete, c.adminEndpoint("users/%s/role-mappings/clients/%s", userID, clientID), token, body); err != nil {
			return err
		}
	}
	role, err := c.getClientRole(ctx, clientID, token, roleName)
	if err != nil {
		return err
	}
	body, _ := JSONBody([]Role{*role})
	_, err = c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("users/%s/role-mappings/clients/%s", userID, clientID), token, body)
	return err
}

func (c *Client) adminContext(ctx context.Context) (clientID, token string, err error) {
	token, err = c.adminToken(ctx)
	if err != nil {
		return "", "", err
	}
	clientID, err = c.clientUUID(ctx, token)
	return clientID, token, err
}

func (c *Client) adminToken(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", c.conf.AdminClientID)
	form.Set("username", c.conf.AdminUsername)
	form.Set("password", c.conf.AdminPassword)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.conf.AdminURL, "/")+"/realms/master/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("keycloak admin token failed: %s", resp.Status)
	}
	var out TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.AccessToken, nil
}

func (c *Client) clientUUID(ctx context.Context, token string) (string, error) {
	return c.clientUUIDByName(ctx, token, c.conf.ClientID)
}

// clientUUIDByName resolves a client's UUID from its client-id string. Used
// for multi-client operations (e.g. managing roles on a client other than the
// configured one, like the advertise service client).
func (c *Client) clientUUIDByName(ctx context.Context, token, clientID string) (string, error) {
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("clients?clientId=%s", url.QueryEscape(clientID)), token, nil)
	if err != nil {
		return "", err
	}
	var clients []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &clients); err != nil {
		return "", err
	}
	if len(clients) == 0 {
		return "", fmt.Errorf("keycloak client not found: %s", clientID)
	}
	return clients[0].ID, nil
}

// EnsureClient creates the given client if it does not already exist. Used by
// the seeder to provision sibling-service clients (e.g. prosky-advertise).
func (c *Client) EnsureClient(ctx context.Context, clientID, secret string) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return err
	}
	return c.ensureClientByName(ctx, token, clientID, secret)
}

func (c *Client) ensureClientByName(ctx context.Context, token, clientID, secret string) error {
	if clientID == "" {
		return fmt.Errorf("keycloak client id is required")
	}
	rep := ClientRepresentation{
		ClientID:                  clientID,
		Name:                      clientID,
		Enabled:                   true,
		Protocol:                  "openid-connect",
		PublicClient:              false,
		Secret:                    secret,
		DirectAccessGrantsEnabled: true,
		ServiceAccountsEnabled:    true,
		StandardFlowEnabled:       true,
		RedirectURIs:              []string{"*"},
		WebOrigins:                []string{"*"},
	}
	if _, err := c.clientUUIDByName(ctx, token, clientID); err == nil {
		return nil
	}
	body, _ := JSONBody(rep)
	_, err := c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("clients"), token, body)
	return err
}

func (c *Client) getClientRole(ctx context.Context, clientID, token, name string) (*Role, error) {
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("clients/%s/roles/%s", clientID, url.PathEscape(name)), token, nil)
	if err != nil {
		return nil, err
	}
	var role Role
	if err := json.Unmarshal(body, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (c *Client) adminRequest(ctx context.Context, method, url, token string, body interface{}) ([]byte, error) {
	reader, _ := body.(interface{ Read([]byte) (int, error) })
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("keycloak admin request failed: %s", resp.Status)
	}
	return out, nil
}

func (c *Client) adminEndpoint(format string, args ...interface{}) string {
	return strings.TrimRight(c.conf.AdminURL, "/") + "/admin/realms/" + c.targetRealm() + "/" + fmt.Sprintf(format, args...)
}

func (c *Client) adminRealmEndpoint(format string, args ...interface{}) string {
	return strings.TrimRight(c.conf.AdminURL, "/") + "/admin/" + fmt.Sprintf(format, args...)
}

func (c *Client) targetRealm() string {
	if c.conf.TargetRealm != "" {
		return c.conf.TargetRealm
	}
	return strings.TrimPrefix(strings.TrimRight(c.conf.RealmURL, "/"), strings.TrimRight(c.conf.AdminURL, "/")+"/realms/")
}
