package authz

import "context"

type contextKey string

const principalKey contextKey = "authz_principal"

type Principal struct {
	LocalUserID string
	KeycloakSub string
	Email       string
	Name        string
	RoleID      string
	RoleSlug    string
	Permissions []string
}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

func HasAll(granted, required []string) bool {
	set := make(map[string]struct{}, len(granted))
	for _, permission := range granted {
		set[permission] = struct{}{}
	}
	for _, permission := range required {
		if _, ok := set[permission]; !ok {
			return false
		}
	}
	return true
}
