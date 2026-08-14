package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ── Realm-level business role management ──
//
// Business roles (super_admin, admin, user, technician, ...) live as REALM
// roles so they are visible to every service in the realm regardless of which
// client issued the token. Permission roles (module.action) stay client-scoped
// and are attached to a business role as cross-client composites.

// UpsertRealmRole creates or updates a realm-level business role.
func (c *Client) UpsertRealmRole(ctx context.Context, name, description string, attributes map[string][]string) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return err
	}
	roleURL := c.adminEndpoint("roles/%s", url.PathEscape(name))
	if _, err := c.adminRequest(ctx, http.MethodGet, roleURL, token, nil); err == nil {
		body, _ := JSONBody(Role{Name: name, Description: description, Attributes: attributes})
		_, err = c.adminRequest(ctx, http.MethodPut, roleURL, token, body)
		return err
	}
	body, _ := JSONBody(Role{Name: name, Description: description, Attributes: attributes})
	_, err = c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("roles"), token, body)
	if err != nil && strings.Contains(err.Error(), "409") {
		return nil
	}
	return err
}

// DeleteRealmRole removes a realm-level business role.
func (c *Client) DeleteRealmRole(ctx context.Context, name string) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.adminRequest(ctx, http.MethodDelete, c.adminEndpoint("roles/%s", url.PathEscape(name)), token, nil)
	return err
}

// ListRealmRolesPublic returns every realm role (briefRepresentation=false so
// attributes are included). Callers filter the slice for business roles.
func (c *Client) ListRealmRolesPublic(ctx context.Context) ([]Role, error) {
	token, err := c.adminToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("roles?briefRepresentation=false"), token, nil)
	if err != nil {
		return nil, err
	}
	var roles []Role
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// GetRealmRolePublic fetches a single realm role by name.
func (c *Client) GetRealmRolePublic(ctx context.Context, name string) (*Role, error) {
	token, err := c.adminToken(ctx)
	if err != nil {
		return nil, err
	}
	return c.getRealmRole(ctx, token, name)
}

func (c *Client) getRealmRole(ctx context.Context, token, name string) (*Role, error) {
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("roles/%s", url.PathEscape(name)), token, nil)
	if err != nil {
		return nil, err
	}
	var role Role
	if err := json.Unmarshal(body, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// GetRealmRoleComposites returns the composite children of a realm role
// (typically permission client-roles attached cross-client).
func (c *Client) GetRealmRoleComposites(ctx context.Context, roleName string) ([]Role, error) {
	token, err := c.adminToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("roles/%s/composites", url.PathEscape(roleName)), token, nil)
	if err != nil {
		return nil, err
	}
	var roles []Role
	_ = json.Unmarshal(body, &roles)
	return roles, nil
}

// SyncCompositeRolesRealmMulti re-points a realm business role's composite
// children to the given permission slugs. Each slug is (re)created on its
// target client and attached to the realm role via roles-by-id (which works
// for both realm and client parents). slugToClient maps permission slug →
// client-id the slug lives on.
func (c *Client) SyncCompositeRolesRealmMulti(ctx context.Context, roleName string, slugToClient map[string]string) error {
	token, err := c.adminToken(ctx)
	if err != nil {
		return err
	}
	role, err := c.getRealmRole(ctx, token, roleName)
	if err != nil {
		return err
	}

	// Ensure each permission role exists on its client and collect its Role rep.
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

	// Fetch ALL current composites of the realm role and delete them before
	// re-adding, so the role reflects exactly slugToClient.
	currentBody, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("roles/%s/composites", url.PathEscape(roleName)), token, nil)
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

// ── User ↔ realm business role mapping ──
//
// Note: AssignRoleToUser (in admin.go) and the user-create helpers attach the
// business role via setUserRealmRole, since business roles are realm roles.

func (c *Client) setUserRealmRole(ctx context.Context, token, userID, roleName string) error {
	currentBody, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("users/%s/role-mappings/realm", userID), token, nil)
	if err != nil {
		return err
	}
	var current []Role
	_ = json.Unmarshal(currentBody, &current)
	remove := make([]Role, 0, len(current))
	for _, role := range current {
		// Only swap business roles; leave built-in realm roles
		// (default-roles-*, offline_access, etc.) untouched.
		if IsBusinessRole(role.Name) {
			remove = append(remove, role)
		}
	}
	if len(remove) > 0 {
		body, _ := JSONBody(remove)
		if _, err := c.adminRequest(ctx, http.MethodDelete, c.adminEndpoint("users/%s/role-mappings/realm", userID), token, body); err != nil {
			return err
		}
	}
	role, err := c.getRealmRole(ctx, token, roleName)
	if err != nil {
		return err
	}
	body, _ := JSONBody([]Role{*role})
	_, err = c.adminRequest(ctx, http.MethodPost, c.adminEndpoint("users/%s/role-mappings/realm", userID), token, body)
	return err
}

// GetUserRealmRolesFull returns the realm roles assigned to a user (raw reps).
func (c *Client) GetUserRealmRolesFull(ctx context.Context, userID string) ([]Role, error) {
	token, err := c.adminToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.adminRequest(ctx, http.MethodGet, c.adminEndpoint("users/%s/role-mappings/realm", userID), token, nil)
	if err != nil {
		return nil, err
	}
	var roles []Role
	_ = json.Unmarshal(body, &roles)
	return roles, nil
}

// ListUsersByRealmRole pages through every user assigned to a realm role.
func (c *Client) ListUsersByRealmRole(ctx context.Context, roleName string) ([]User, error) {
	token, err := c.adminToken(ctx)
	if err != nil {
		return nil, err
	}
	var all []User
	const pageSize = 100
	for first := 0; ; first += pageSize {
		endpoint := c.adminEndpoint("roles/%s/users?first=%d&max=%d",
			url.PathEscape(roleName), first, pageSize)
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

// RevokeRealmRoleUserSessions revokes the active sessions of every user holding
// a realm business role (concurrent, best-effort). Mirrors
// RevokeRoleUserSessions but enumerates via the realm role.
func (c *Client) RevokeRealmRoleUserSessions(ctx context.Context, roleName string) (revoked, failed int, err error) {
	users, err := c.ListUsersByRealmRole(ctx, roleName)
	if err != nil {
		return 0, 0, fmt.Errorf("list users by realm role %s: %w", roleName, err)
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

// RemoveClientBusinessRoleFromUser detaches a client business role from a user.
// Used by the one-off migration when moving users off the legacy client roles.
func (c *Client) RemoveClientBusinessRoleFromUser(ctx context.Context, userID, roleName string) error {
	clientID, token, err := c.adminContext(ctx)
	if err != nil {
		return err
	}
	role, err := c.getClientRole(ctx, clientID, token, roleName)
	if err != nil {
		return err
	}
	body, _ := JSONBody([]Role{*role})
	_, err = c.adminRequest(ctx, http.MethodDelete, c.adminEndpoint("users/%s/role-mappings/clients/%s", userID, clientID), token, body)
	return err
}
