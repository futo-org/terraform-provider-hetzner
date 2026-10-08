// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

func TestUnitGetWithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/test" {
			t.Errorf("expected path /test, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	c := NewClient("user", "pass")
	c.BaseURL = server.URL

	body, err := c.GetWithContext(context.Background(), "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestUnitPostWithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if r.PostFormValue("key") != "value" {
			t.Errorf("expected form key=value, got key=%s", r.PostFormValue("key"))
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"created":true}`)
	}))
	defer server.Close()

	c := NewClient("user", "pass")
	c.BaseURL = server.URL

	data := url.Values{"key": {"value"}}
	body, err := c.PostWithContext(context.Background(), "/test", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != `{"created":true}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestUnitContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler should not complete because the context is cancelled.
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	c := NewClient("user", "pass")
	c.BaseURL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := c.GetWithContext(ctx, "/test")
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

func TestUnitGetWithContextBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Error("expected basic auth to be set")
		}
		if user != "testuser" || pass != "testpass" {
			t.Errorf("expected testuser:testpass, got %s:%s", user, pass)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	c := NewClient("testuser", "testpass")
	c.BaseURL = server.URL

	_, err := c.GetWithContext(context.Background(), "/auth-check")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnitGetWithContextAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"status":404,"code":"NOT_FOUND","message":"resource not found"}}`)
	}))
	defer server.Close()

	c := NewClient("user", "pass")
	c.BaseURL = server.URL

	_, err := c.GetWithContext(context.Background(), "/missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
	if apiErr.ErrorCode != "NOT_FOUND" {
		t.Errorf("expected error code NOT_FOUND, got %s", apiErr.ErrorCode)
	}
}

func TestUnitGetCached(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		n := hits
		mu.Unlock()
		fmt.Fprintf(w, `{"read":%d}`, n)
	}))
	defer server.Close()

	c := NewClient("user", "pass")
	c.BaseURL = server.URL

	var wg sync.WaitGroup
	bodies := make([]string, 20)
	for i := range bodies {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body, err := c.GetCached("/server")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			bodies[i] = string(body)
		}(i)
	}
	wg.Wait()

	if hits != 1 {
		t.Fatalf("expected 1 request for 20 concurrent reads, got %d", hits)
	}
	for _, b := range bodies {
		if b != `{"read":1}` {
			t.Fatalf("unexpected body: %s", b)
		}
	}

	c.ForgetCached("/server")
	body, err := c.GetCached("/server")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != `{"read":2}` || hits != 2 {
		t.Fatalf("expected a fresh read after ForgetCached, got %s after %d requests", body, hits)
	}
}
