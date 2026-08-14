package keycloak

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestValidClient(t *testing.T) {
	c := New(AccountConfig{ClientID: "prosky-admin"})
	tests := []struct {
		name   string
		claims jwt.MapClaims
		want   bool
	}{
		{name: "azp matches", claims: jwt.MapClaims{"azp": "prosky-admin"}, want: true},
		{name: "string audience matches", claims: jwt.MapClaims{"aud": "prosky-admin"}, want: true},
		{name: "array audience matches", claims: jwt.MapClaims{"aud": []interface{}{"account", "prosky-admin"}}, want: true},
		{name: "resource access matches", claims: jwt.MapClaims{"resource_access": map[string]interface{}{"prosky-admin": map[string]interface{}{}}}, want: true},
		{name: "wrong client rejected", claims: jwt.MapClaims{"azp": "other-client", "aud": []interface{}{"account"}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.validClient(tt.claims); got != tt.want {
				t.Fatalf("validClient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractClaimsSplitsBusinessRoleAndPermissions(t *testing.T) {
	c := New(AccountConfig{ClientID: "prosky-admin"})
	claims := c.extractClaims(jwt.MapClaims{
		"sub":   "kc-user-id",
		"email": "admin@example.com",
		"name":  "Admin User",
		"resource_access": map[string]interface{}{
			"prosky-admin": map[string]interface{}{
				"roles": []interface{}{"admin", "users.read", "roles.read"},
			},
		},
	})
	if claims.RoleSlug != "admin" {
		t.Fatalf("RoleSlug = %q, want admin", claims.RoleSlug)
	}
	if len(claims.Permissions) != 2 || claims.Permissions[0] != "users.read" || claims.Permissions[1] != "roles.read" {
		t.Fatalf("Permissions = %#v, want users.read and roles.read", claims.Permissions)
	}
}

func TestExtractClaimsSuperAdminWins(t *testing.T) {
	c := New(AccountConfig{ClientID: "prosky-admin"})
	claims := c.extractClaims(jwt.MapClaims{
		"resource_access": map[string]interface{}{
			"prosky-admin": map[string]interface{}{
				"roles": []interface{}{"admin", "super_admin", "users.read"},
			},
		},
	})
	if claims.RoleSlug != SuperAdminSlug {
		t.Fatalf("RoleSlug = %q, want %s", claims.RoleSlug, SuperAdminSlug)
	}
}

// Realm-role model: business role lives in realm_access.roles and must be
// visible to every service regardless of the configured client. Permissions
// stay client-scoped in resource_access.
func TestExtractClaimsReadsRealmBusinessRole(t *testing.T) {
	c := New(AccountConfig{ClientID: "prosky-advertise"})
	claims := c.extractClaims(jwt.MapClaims{
		"sub": "kc-user-id",
		"realm_access": map[string]interface{}{
			"roles": []interface{}{"super_admin", "default-roles-prosky-dev"},
		},
		"resource_access": map[string]interface{}{
			"prosky-advertise": map[string]interface{}{
				"roles": []interface{}{"videos.create", "schedules.read"},
			},
		},
	})
	if claims.RoleSlug != SuperAdminSlug {
		t.Fatalf("RoleSlug = %q, want %s (super_admin must be visible across services via realm_access)", claims.RoleSlug, SuperAdminSlug)
	}
	if len(claims.Permissions) != 2 {
		t.Fatalf("Permissions = %#v, want 2 client-scoped permissions", claims.Permissions)
	}
}

func TestExtractClaimsRealmRoleFallbackToClient(t *testing.T) {
	c := New(AccountConfig{ClientID: "prosky-admin"})
	// Legacy token (pre-migration): no realm_access, business role still in client roles.
	claims := c.extractClaims(jwt.MapClaims{
		"resource_access": map[string]interface{}{
			"prosky-admin": map[string]interface{}{
				"roles": []interface{}{"admin", "users.read"},
			},
		},
	})
	if claims.RoleSlug != "admin" {
		t.Fatalf("RoleSlug = %q, want admin (legacy client-role fallback)", claims.RoleSlug)
	}
}
