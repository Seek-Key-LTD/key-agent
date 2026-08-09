// Authentik Bearer-token 校验中间件（A2A 服务端认证）
// 校验 incoming Bearer token 是否有效：调 authentik userinfo 端点。
// 通过后把调用者身份写入 context（caller_identity）。
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// UserInfo 是 authentik userinfo 响应的最小子集。
type UserInfo struct {
	Sub               string `json:"sub"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
}

type ctxKey string

const callerIdentityKey ctxKey = "caller_identity"

// CallerIdentity 从请求 context 里取调用者身份（Authentik 中间件注入）。
// 未认证请求返回空串。
func CallerIdentity(ctx context.Context) string {
	if v, ok := ctx.Value(callerIdentityKey).(string); ok {
		return v
	}
	return ""
}

// AuthentikUserInfoMiddleware 校验 Authorization: Bearer <token>，
// 调 authentik userinfo 端点验真，通过后把 preferred_username 注入 context。
// userInfoURL 通常为 https://<authentik>/application/o/userinfo/。
func AuthentikUserInfoMiddleware(userInfoURL string, next http.Handler) http.Handler {
	client := &http.Client{Timeout: 5 * time.Second}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"Missing or malformed Authorization header"}`, http.StatusUnauthorized)
			return
		}

		userInfoReq, err := http.NewRequestWithContext(r.Context(), "GET", userInfoURL, nil)
		if err != nil {
			http.Error(w, `{"error":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		userInfoReq.Header.Set("Authorization", authHeader)

		resp, err := client.Do(userInfoReq)
		if err != nil || resp.StatusCode != http.StatusOK {
			http.Error(w, `{"error":"Invalid or unauthenticated Authentik token"}`, http.StatusUnauthorized)
			if resp != nil {
				resp.Body.Close()
			}
			return
		}
		defer resp.Body.Close()

		var ui UserInfo
		if err := json.NewDecoder(resp.Body).Decode(&ui); err != nil {
			http.Error(w, `{"error":"Failed to parse UserInfo"}`, http.StatusBadRequest)
			return
		}

		ctx := context.WithValue(r.Context(), callerIdentityKey, ui.PreferredUsername)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
