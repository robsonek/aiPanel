package httpserver

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestUseSecureCookie(t *testing.T) {
	t.Run("prod over plain http", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.test", nil)
		if useSecureCookie("prod", req) {
			t.Fatal("expected secure cookie disabled for plain http request")
		}
	})

	t.Run("prod with tls", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.test", nil)
		req.TLS = &tls.ConnectionState{}
		if !useSecureCookie("prod", req) {
			t.Fatal("expected secure cookie enabled for tls request")
		}
	})

	t.Run("prod behind reverse proxy https", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.test", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		if !useSecureCookie("prod", req) {
			t.Fatal("expected secure cookie enabled for x-forwarded-proto=https")
		}
	})

	t.Run("prod behind reverse proxy mixed list", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.test", nil)
		req.Header.Set("X-Forwarded-Proto", "https,http")
		if !useSecureCookie("prod", req) {
			t.Fatal("expected secure cookie enabled when first forwarded proto is https")
		}
	})

	t.Run("dev over https", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://example.test", nil)
		req.TLS = &tls.ConnectionState{}
		if useSecureCookie("dev", req) {
			t.Fatal("expected secure cookie disabled in dev env")
		}
	})
}
