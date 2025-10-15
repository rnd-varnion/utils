package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rnd-varnion/utils/authentication"
	"github.com/rnd-varnion/utils/permission"
	"github.com/rnd-varnion/utils/tools"
)

func Permission(app, menu, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		isFromInternal := c.GetBool(authentication.IsFromInternalKey)
		if isFromInternal {
			// If request is from internal, skip permission check
			c.Next()
			return
		}

		userSessionData := c.GetString(authentication.SessionKey)
		if userSessionData == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Session not found",
			})
			return
		}

		var userSession authentication.UserSession
		err := json.Unmarshal([]byte(userSessionData), &userSession)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Failed to bind user session data",
			})
			return
		}

		// Check if the required app, menu, and permission exist in the claims
		hasPermission := false
		for _, permToken := range userSession.Permissions {
			if strings.EqualFold(permToken.AppName, app) {
				for _, menuItem := range permToken.Menus {
					if strings.EqualFold(menuItem.MenuName, menu) {
						for _, perm := range menuItem.Permission {
							if perm != nil && *perm == permission {
								hasPermission = true
								break
							}
						}
						if hasPermission {
							break
						}
					}
				}
				if hasPermission {
					break
				}
			}
		}

		if !hasPermission {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Insufficient permissions for this resource",
			})
			return
		}

		// Validate Success
		c.Next()
	}
}

func PermissionBulk(payload []permission.BulkPayload) gin.HandlerFunc {
	return func(c *gin.Context) {
		isFromInternal := c.GetBool(authentication.IsFromInternalKey)
		if isFromInternal {
			// If request is from internal, skip permission check
			c.Next()
			return
		}

		// Logic Permission
		userSessionData := c.GetString(authentication.SessionKey)
		if userSessionData == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Session not found",
			})
			return
		}

		var userSession authentication.UserSession
		err := json.Unmarshal([]byte(userSessionData), &userSession)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Failed to bind user session data",
			})
			return
		}

		// Check if the required app, menu, and permission exist in the claims
		hasPermission := false
		for _, payloadItem := range payload {
			for _, permToken := range userSession.Permissions {
				if strings.EqualFold(permToken.AppName, payloadItem.App) {
					for _, menuItem := range permToken.Menus {
						if strings.EqualFold(menuItem.MenuName, payloadItem.Menu) {
							for _, perm := range menuItem.Permission {
								if perm != nil && *perm == payloadItem.Permission {
									hasPermission = true
									break
								}
							}
							if hasPermission {
								break
							}
						}
					}
					if hasPermission {
						break
					}
				}
				if hasPermission {
					break
				}
			}
			if hasPermission {
				break
			}
		}

		if !hasPermission {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Insufficient permissions for this resource",
			})
			return
		}

		// Validate Success
		c.Next()
	}
}
