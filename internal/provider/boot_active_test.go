// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeBoot serves one /boot/123/<kind> configuration whose activation the test
// can consume, as booting the server does, and records every write.
type fakeBoot struct {
	mu     sync.Mutex
	kind   string
	active bool
	writes []string
}

func (f *fakeBoot) consume() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = false
}

func (f *fakeBoot) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

func (f *fakeBoot) server(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/boot/123/"+f.kind {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.mu.Lock()
		switch r.Method {
		case http.MethodPost:
			f.active = true
			f.writes = append(f.writes, r.Method)
		case http.MethodDelete:
			f.active = false
			f.writes = append(f.writes, r.Method)
		}
		active := f.active
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			f.kind: map[string]interface{}{
				"server_ip": "1.2.3.4", "server_ipv6_net": "2a01:4f8::/64", "server_number": 123,
				"os": "linux", "dist": "Debian 13 base", "lang": "en", "active": active,
			},
		})
	}))
}

func checkBootWrites(f *fakeBoot, want int) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if got := f.writeCount(); got != want {
			return fmt.Errorf("%d writes to /boot/123/%s, want %d (%v)", got, f.kind, want, f.writes)
		}
		return nil
	}
}

func testBootUpdateAfterBoot(t *testing.T, kind, config string) {
	f := &fakeBoot{kind: kind}
	ts := f.server(t)
	defer ts.Close()

	name := "hetzner_boot_" + kind + ".test"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(config, `["aa:bb"]`),
				Check:  checkBootWrites(f, 1),
			},
			{
				Config: fmt.Sprintf(config, `["aa:bb", "cc:dd"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkBootWrites(f, 3),
					resource.TestCheckResourceAttr(name, "active", "true"),
				),
			},
			{
				PreConfig: f.consume,
				Config:    fmt.Sprintf(config, `["cc:dd"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkBootWrites(f, 3),
					resource.TestCheckResourceAttr(name, "active", "false"),
					resource.TestCheckResourceAttr(name, "authorized_keys.0", "cc:dd"),
				),
			},
		},
	})
}

func TestUnitBootRescueResource_UpdateAfterBootDoesNotRearm(t *testing.T) {
	testBootUpdateAfterBoot(t, "rescue", `
resource "hetzner_boot_rescue" "test" {
  server_number   = 123
  os              = "linux"
  authorized_keys = %s
}`)
}

func TestUnitBootLinuxResource_UpdateAfterBootDoesNotRearm(t *testing.T) {
	testBootUpdateAfterBoot(t, "linux", `
resource "hetzner_boot_linux" "test" {
  server_number   = 123
  dist            = "Debian 13 base"
  lang            = "en"
  authorized_keys = %s
}`)
}
