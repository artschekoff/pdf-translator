package quota

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func Middleware(client *Client, scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		uid, _ := userID.(string)

		result, err := client.Increment(c.Request.Context(), uid, scope)
		if err != nil {
			c.Next()
			return
		}
		if !result.Accepted {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":     "quota exceeded",
				"scope":     scope,
				"remaining": 0,
				"reset_at":  result.Status.ResetAt,
			})
			return
		}
		c.Set("quota_remaining", result.Status.Remaining)
		c.Next()
	}
}
