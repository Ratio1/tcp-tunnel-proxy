package routeprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPProviderCachesSuccessfulLookups(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if got := r.URL.Query().Get("public_port"); got != "30001" {
			t.Errorf("public_port query = %q, want 30001", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "origin.ratio1.link"})
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPProvider returned error: %v", err)
	}

	for i := 0; i < 2; i++ {
		hostname, err := provider.GetHostname(context.Background(), 30001)
		if err != nil {
			t.Fatalf("GetHostname returned error: %v", err)
		}
		if hostname != "origin.ratio1.link" {
			t.Fatalf("hostname = %q, want origin.ratio1.link", hostname)
		}
	}

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("route lookup hits = %d, want 1", got)
	}
}

func TestHTTPProviderAcceptsRawJSONString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode("origin.ratio1.link")
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPProvider returned error: %v", err)
	}

	hostname, err := provider.GetHostname(context.Background(), 30002)
	if err != nil {
		t.Fatalf("GetHostname returned error: %v", err)
	}
	if hostname != "origin.ratio1.link" {
		t.Fatalf("hostname = %q, want origin.ratio1.link", hostname)
	}
}

func TestHTTPProviderDoesNotCacheFailures(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&hits, 1)
		if count == 1 {
			http.Error(w, "missing route", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "origin.ratio1.link"})
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPProvider returned error: %v", err)
	}

	if _, err := provider.GetHostname(context.Background(), 30003); err == nil {
		t.Fatalf("expected first lookup to fail")
	}
	hostname, err := provider.GetHostname(context.Background(), 30003)
	if err != nil {
		t.Fatalf("second GetHostname returned error: %v", err)
	}
	if hostname != "origin.ratio1.link" {
		t.Fatalf("hostname = %q, want origin.ratio1.link", hostname)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("route lookup hits = %d, want 2", got)
	}
}

func TestHTTPProviderDoesNotCacheInvalidHostnames(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&hits, 1)
		if count == 1 {
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "localhost"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "origin.ratio1.link"})
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, time.Second)
	if err != nil {
		t.Fatalf("NewHTTPProvider returned error: %v", err)
	}

	if _, err := provider.GetHostname(context.Background(), 30004); err == nil {
		t.Fatalf("expected invalid hostname response to fail")
	}
	hostname, err := provider.GetHostname(context.Background(), 30004)
	if err != nil {
		t.Fatalf("second GetHostname returned error: %v", err)
	}
	if hostname != "origin.ratio1.link" {
		t.Fatalf("hostname = %q, want origin.ratio1.link", hostname)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("route lookup hits = %d, want 2", got)
	}
}

func TestHTTPProviderRejectsInvalidInputs(t *testing.T) {
	if _, err := NewHTTPProvider("not a url", time.Second); err == nil {
		t.Fatalf("expected invalid base URL error")
	}
	provider, err := NewHTTPProvider("https://example.com", time.Second)
	if err != nil {
		t.Fatalf("NewHTTPProvider returned error: %v", err)
	}
	for _, port := range []int{0, -1, 65536} {
		t.Run(strconv.Itoa(port), func(t *testing.T) {
			if _, err := provider.GetHostname(context.Background(), port); err == nil {
				t.Fatalf("expected invalid port error")
			}
		})
	}
}
