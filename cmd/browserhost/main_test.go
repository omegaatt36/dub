//go:build browser

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListenRequiresNumericLoopback(t *testing.T) {
	for _, address := range []string{":0", "localhost:0", "0.0.0.0:0", "192.0.2.1:0"} {
		listener, err := listenLoopback(address)
		if err == nil {
			listener.Close()
			t.Fatalf("accepted nonnumeric or nonloopback address %q", address)
		}
	}
	listener, err := listenLoopback("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if strings.HasSuffix(listener.Addr().String(), ":0") {
		t.Fatal("listener did not select an actual port")
	}
}

func TestBrowserHostRejectsCrossOriginAndRebindingBeforeAPI(t *testing.T) {
	const authority = "127.0.0.1:12345"
	for _, test := range []struct {
		name, host, origin, site string
		allowed                  bool
	}{
		{"same origin", authority, "http://" + authority, "same-origin", true},
		{"same origin legacy", authority, "http://" + authority, "", true},
		{"nonbrowser", authority, "", "", true},
		{"cross origin", authority, "https://attacker.invalid", "", false},
		{"cross site", authority, "", "cross-site", false},
		{"opaque origin", authority, "null", "", false},
		{"rebound host", "attacker.invalid:12345", "", "same-origin", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			handler := browserHandler(api, t.TempDir(), authority)
			request := httptest.NewRequest(http.MethodPost, "http://"+test.host+"/api/scan", nil)
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Sec-Fetch-Site", test.site)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if called != test.allowed {
				t.Fatalf("API called = %v, want %v", called, test.allowed)
			}
			if !test.allowed && response.Code != http.StatusForbidden {
				t.Fatalf("unsafe request returned %d, want 403", response.Code)
			}
		})
	}
}
