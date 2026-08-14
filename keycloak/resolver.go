package keycloak

import (
	"context"
	"strings"
	"time"
)

// CacheStore is the minimal cache surface UserResolver needs. Both services'
// cache implementations (be-admin pkg/cache and be-advertise pkg/infra/cache)
// satisfy it structurally via GetJSON/SetJSON, so the shared module never
// imports a service's cache package.
type CacheStore interface {
	GetJSON(ctx context.Context, key string, dst interface{}) (bool, error)
	SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
}

type UserInfo struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type UserResolver struct {
	KC    *Client
	Cache CacheStore
}

func NewUserResolver(kc *Client, ch CacheStore) *UserResolver {
	return &UserResolver{KC: kc, Cache: ch}
}

// ResolveUsers batch-resolves keycloak subs to {name, email} via admin API.
// Uses Redis cache (5 min TTL) to avoid repeated lookups for the same user.
// Errors for individual users are silently skipped (returns empty UserInfo).
func (r *UserResolver) ResolveUsers(ctx context.Context, userIDs []string) map[string]UserInfo {
	result := make(map[string]UserInfo, len(userIDs))
	seen := map[string]bool{}

	var missing []string
	for _, id := range userIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		if r.Cache != nil {
			var cached UserInfo
			if ok, _ := r.Cache.GetJSON(ctx, "kcuser:"+id, &cached); ok {
				result[id] = cached
				continue
			}
		}
		missing = append(missing, id)
	}

	for _, id := range missing {
		user, err := r.KC.GetUserByID(ctx, id)
		if err != nil || user == nil {
			result[id] = UserInfo{ID: id}
			continue
		}
		name := strings.TrimSpace(user.FirstName + " " + user.LastName)
		info := UserInfo{ID: user.ID, Email: user.Email, Name: name}
		result[id] = info
		if r.Cache != nil {
			_, _ = r.Cache.SetJSON(ctx, "kcuser:"+id, info, 5*time.Minute)
		}
	}

	return result
}

// ResolveOne resolves a single user. Shortcut for ResolveUsers with one ID.
func (r *UserResolver) ResolveOne(ctx context.Context, userID string) UserInfo {
	m := r.ResolveUsers(ctx, []string{userID})
	if info, ok := m[userID]; ok {
		return info
	}
	return UserInfo{ID: userID}
}
