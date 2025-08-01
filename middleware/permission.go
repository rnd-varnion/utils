package middleware

import (
	"net/http"

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

		// Logic Permission
		permissionHeader := c.GetHeader("Permission-Token")
		if permissionHeader == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Permission token is required",
			})
			return
		}

		permissionClaim, err := tools.ValidatePermissionToken(permissionHeader)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Invalid or expired permission token",
			})
			return
		}

		// Check if the required app, menu, and permission exist in the claims
		hasPermission := false
		for _, permToken := range permissionClaim.Data {
			if permToken.AppName == app {
				for _, menuItem := range permToken.Menus {
					if menuItem.MenuName == menu {
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
		permissionHeader := c.GetHeader("Permission-Token")
		if permissionHeader == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Permission token is required",
			})
			return
		}

		permissionClaim, err := tools.ValidatePermissionToken(permissionHeader)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, tools.Response{
				Status:  "Forbidden",
				Message: "Invalid or expired permission token",
			})
			return
		}

		// Check if the required app, menu, and permission exist in the claims
		hasPermission := false
		for _, payloadItem := range payload {
			for _, permToken := range permissionClaim.Data {
				if permToken.AppName == payloadItem.App {
					for _, menuItem := range permToken.Menus {
						if menuItem.MenuName == payloadItem.Menu {
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
