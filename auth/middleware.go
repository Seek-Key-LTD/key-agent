// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Authentik Bearer-token 校验中间件（A2A 服务端认证）。
//
// Authentik 的 M2M token 是 HS256（对称）签名：收方持有的是别人的 client_secret
// 拿不到，只有 Authentik 自己能验，因此 userinfo introspection 是正确的验真原语，
// 不能换成本地 JWKS 验签。问题只在"每个请求都同步打 Authentik"——IdP 一挂，全网状
// A2A 同时瞎（相关性故障 / SPOF）。所以这里对 userinfo 的正向结果做一层进程内短 TTL
// 缓存：命中就不再出网，IdP 抖动时已暖的 token 不掉线。缓存只存身份、以 token 的
// SHA-256 为键（绝不落原文 token），并额外用 token 自带的 exp 收紧上限，杜绝缓存放行
// 过期身份。冷路径（无缓存且 IdP 不可达）返回 503，与"token 无效"的 401 区分开。
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
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

// defaultUserinfoCacheTTL 远小于 Authentik client_credentials token 的 600s 生命期，
// 把"缓存放行已撤销 token"的窗口压到一秒级抖动可接受的程度。
const defaultUserinfoCacheTTL = 60 * time.Second

// CallerIdentity 从请求 context 里取调用者身份（Authentik 中间件注入）。
// 未认证请求返回空串。
func CallerIdentity(ctx context.Context) string {
	if v, ok := ctx.Value(callerIdentityKey).(string); ok {
		return v
	}
	return ""
}

// cacheEntry 是一条已验真的 userinfo 正向结果。
type cacheEntry struct {
	user     string
	expireAt time.Time
}

// verifyCache 缓存 userinfo 正向结果，键为 bearer token 的 SHA-256。
// ttl<=0 时退化为不缓存（等价旧行为），用于测试与回滚。
type verifyCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]cacheEntry
	now     func() time.Time
}

func newVerifyCache(ttl time.Duration) *verifyCache {
	return &verifyCache{ttl: ttl, entries: make(map[string]cacheEntry), now: time.Now}
}

func (c *verifyCache) get(key string) (string, bool) {
	if c.ttl <= 0 {
		return "", false
	}
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if c.now().After(e.expireAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return "", false
	}
	return e.user, true
}

// put 写入正向结果；hardExpire（token 自带 exp）非零时收紧缓存上限，
// 保证缓存不会比 token 本身活得更久。
func (c *verifyCache) put(key, user string, hardExpire time.Time) {
	if c.ttl <= 0 {
		return
	}
	now := c.now()
	expire := now.Add(c.ttl)
	if !hardExpire.IsZero() && hardExpire.Before(expire) {
		expire = hardExpire
	}
	if !now.Before(expire) {
		return // 已过期，不缓存
	}
	c.mu.Lock()
	c.entries[key] = cacheEntry{user: user, expireAt: expire}
	if len(c.entries) > 1024 { // 机会式清理，给内存一个上界
		for k, v := range c.entries {
			if now.After(v.expireAt) {
				delete(c.entries, k)
			}
		}
	}
	c.mu.Unlock()
}

// tokenHardExpiry 只解 JWT payload 的 exp 用于收紧缓存，不校验签名、绝不作为放行依据。
// 解析失败返回零值。
func tokenHardExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		if raw, err = base64.URLEncoding.DecodeString(parts[1]); err != nil {
			return time.Time{}
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// AuthentikUserInfoMiddleware 校验 Authorization: Bearer <token>，
// 调 authentik userinfo 端点验真，通过后把 preferred_username 注入 context。
// userInfoURL 通常为 https://<authentik>/application/o/userinfo/。
// 默认带 60s 正向缓存以消除逐请求 IdP 依赖。
func AuthentikUserInfoMiddleware(userInfoURL string, next http.Handler) http.Handler {
	return AuthentikUserInfoMiddlewareWithCache(userInfoURL, defaultUserinfoCacheTTL, next)
}

// AuthentikUserInfoMiddlewareWithCache 同 AuthentikUserInfoMiddleware，但可指定
// 缓存 TTL；ttl<=0 关闭缓存（逐请求 introspection）。
func AuthentikUserInfoMiddlewareWithCache(userInfoURL string, ttl time.Duration, next http.Handler) http.Handler {
	client := &http.Client{Timeout: 5 * time.Second}
	cache := newVerifyCache(ttl)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"Missing or malformed Authorization header"}`, http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		key := hashToken(token)

		if user, ok := cache.get(key); ok {
			ctx := context.WithValue(r.Context(), callerIdentityKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		userInfoReq, err := http.NewRequestWithContext(r.Context(), "GET", userInfoURL, nil)
		if err != nil {
			http.Error(w, `{"error":"Internal error"}`, http.StatusInternalServerError)
			return
		}
		userInfoReq.Header.Set("Authorization", authHeader)

		resp, err := client.Do(userInfoReq)
		if err != nil {
			// IdP 不可达且无缓存身份：以"服务不可用"失败，与 token 无效(401)区分。
			http.Error(w, `{"error":"Identity provider unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			http.Error(w, `{"error":"Invalid or unauthenticated Authentik token"}`, http.StatusUnauthorized)
			return
		}

		var ui UserInfo
		if err := json.NewDecoder(resp.Body).Decode(&ui); err != nil {
			http.Error(w, `{"error":"Failed to parse UserInfo"}`, http.StatusBadRequest)
			return
		}

		cache.put(key, ui.PreferredUsername, tokenHardExpiry(token))
		ctx := context.WithValue(r.Context(), callerIdentityKey, ui.PreferredUsername)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
