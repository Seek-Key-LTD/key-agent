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

package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/adk/v2/auth"
)

// makeJWT 构造一个仅用于测试的三段式 JWT（签名位是假的），payload 里可塞 exp。
// 中间件只 best-effort 解 exp 收紧缓存，从不验签，故假签名不影响。
func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return "eyJhbGciOiJIUzI1NiJ9." + payload + ".c2ln"
}

// newCountingUserInfo 返回一个记录被调用次数的 userinfo 测试服务。
// ok 控制它是 200(带 preferred_username) 还是 401；reachable=false 时直接关闭端口。
func newCountingUserInfo(t *testing.T, user string, ok bool) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"sub":                "sub-" + user,
			"preferred_username": user,
			"name":               user,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// newProbe 返回一个记录下游是否被调用及注入身份的 next handler。
func newProbe() (http.Handler, *bool, *string) {
	called := new(bool)
	id := new(string)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		*id = auth.CallerIdentity(r.Context())
		w.WriteHeader(http.StatusTeapot)
	})
	return h, called, id
}

func doReq(h http.Handler, bearer string) int {
	req := httptest.NewRequest("POST", "/a2a", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestUserInfoMiddleware_MissingHeader(t *testing.T) {
	srv, calls := newCountingUserInfo(t, "amber", true)
	probe, called, _ := newProbe()
	mw := auth.AuthentikUserInfoMiddleware(srv.URL, probe)

	if code := doReq(mw, ""); code != http.StatusUnauthorized {
		t.Fatalf("missing header: want 401, got %d", code)
	}
	if *called {
		t.Error("downstream must not run without a bearer token")
	}
	if calls.Load() != 0 {
		t.Errorf("IdP must not be consulted for missing header, calls=%d", calls.Load())
	}
}

func TestUserInfoMiddleware_ValidToken(t *testing.T) {
	srv, _ := newCountingUserInfo(t, "topaz", true)
	probe, called, id := newProbe()
	mw := auth.AuthentikUserInfoMiddleware(srv.URL, probe)

	if code := doReq(mw, "tok-valid"); code != http.StatusTeapot {
		t.Fatalf("valid: want 200 downstream, got %d", code)
	}
	if !*called || *id != "topaz" {
		t.Errorf("want downstream called with identity topaz, got called=%v id=%q", *called, *id)
	}
}

func TestUserInfoMiddleware_InvalidToken401(t *testing.T) {
	srv, calls := newCountingUserInfo(t, "x", false) // always 401
	probe, called, _ := newProbe()
	mw := auth.AuthentikUserInfoMiddleware(srv.URL, probe)

	if code := doReq(mw, "tok-bad"); code != http.StatusUnauthorized {
		t.Fatalf("invalid: want 401, got %d", code)
	}
	if *called {
		t.Error("downstream must not run for invalid token")
	}
	// 负向结果不缓存：第二次仍应打到 IdP。
	doReq(mw, "tok-bad")
	if calls.Load() != 2 {
		t.Errorf("invalid token must not be cached, IdP calls=%d want 2", calls.Load())
	}
}

func TestUserInfoMiddleware_CacheHitSkipsIdP(t *testing.T) {
	srv, calls := newCountingUserInfo(t, "ruby", true)
	probe, called, id := newProbe()
	mw := auth.AuthentikUserInfoMiddlewareWithCache(srv.URL, time.Minute, probe)

	tok := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	if code := doReq(mw, tok); code != http.StatusTeapot {
		t.Fatalf("first: want 200, got %d", code)
	}
	*called, *id = false, ""
	if code := doReq(mw, tok); code != http.StatusTeapot {
		t.Fatalf("second: want 200 from cache, got %d", code)
	}
	if !*called || *id != "ruby" {
		t.Errorf("cache path must inject identity, called=%v id=%q", *called, *id)
	}
	if calls.Load() != 1 {
		t.Errorf("repeat request must hit IdP once, calls=%d want 1", calls.Load())
	}
}

// SPOF 消除的正面证明：暖缓存后即便 IdP 挂掉，已验真的 token 仍可通行。
func TestUserInfoMiddleware_IdPDownButCached(t *testing.T) {
	srv, _ := newCountingUserInfo(t, "agate", true)
	probe, called, id := newProbe()
	mw := auth.AuthentikUserInfoMiddlewareWithCache(srv.URL, time.Minute, probe)

	tok := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	if code := doReq(mw, tok); code != http.StatusTeapot {
		t.Fatalf("warm-up: want 200, got %d", code)
	}
	srv.Close() // IdP 下线

	*called, *id = false, ""
	if code := doReq(mw, tok); code != http.StatusTeapot {
		t.Fatalf("cached while IdP down: want 200, got %d", code)
	}
	if !*called || *id != "agate" {
		t.Errorf("cached identity must survive IdP outage, called=%v id=%q", *called, *id)
	}
}

// 冷路径（无缓存 + IdP 不可达）必须返回 503，区别于 401。
func TestUserInfoMiddleware_IdPDownColdFail503(t *testing.T) {
	srv, _ := newCountingUserInfo(t, "x", true)
	url := srv.URL
	srv.Close() // 端口关闭 → 传输错误
	probe, called, _ := newProbe()
	mw := auth.AuthentikUserInfoMiddleware(url, probe)

	if code := doReq(mw, "tok-cold"); code != http.StatusServiceUnavailable {
		t.Fatalf("cold + down: want 503, got %d", code)
	}
	if *called {
		t.Error("downstream must not run when IdP is unreachable with no cache")
	}
}

// 过期后必须重新 introspection。
func TestUserInfoMiddleware_CacheExpiry(t *testing.T) {
	srv, calls := newCountingUserInfo(t, "amber", true)
	probe, _, _ := newProbe()
	mw := auth.AuthentikUserInfoMiddlewareWithCache(srv.URL, 30*time.Millisecond, probe)

	tok := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	doReq(mw, tok)
	time.Sleep(60 * time.Millisecond)
	doReq(mw, tok)
	if calls.Load() != 2 {
		t.Errorf("expired entry must re-fetch, calls=%d want 2", calls.Load())
	}
}

// token 自带 exp 已过期时，即使 TTL 更长也不得缓存（放行窗口不能超寿命）。
func TestUserInfoMiddleware_ExpCapNotCached(t *testing.T) {
	srv, calls := newCountingUserInfo(t, "amber", true)
	probe, _, _ := newProbe()
	mw := auth.AuthentikUserInfoMiddlewareWithCache(srv.URL, time.Hour, probe)

	tok := makeJWT(t, map[string]any{"exp": time.Now().Add(-time.Second).Unix()})
	doReq(mw, tok)
	doReq(mw, tok)
	if calls.Load() != 2 {
		t.Errorf("already-expired token must not be cached, calls=%d want 2", calls.Load())
	}
}

// 无 exp 的 token（解析不出）仍按 TTL 正常缓存，不因解不到 exp 而拒绝缓存。
func TestUserInfoMiddleware_NoExpStillCaches(t *testing.T) {
	srv, calls := newCountingUserInfo(t, "amber", true)
	probe, _, _ := newProbe()
	mw := auth.AuthentikUserInfoMiddlewareWithCache(srv.URL, time.Minute, probe)

	opaque := "not-a-jwt"
	doReq(mw, opaque)
	doReq(mw, opaque)
	if calls.Load() != 1 {
		t.Errorf("opaque token should cache by TTL, calls=%d want 1", calls.Load())
	}
}
