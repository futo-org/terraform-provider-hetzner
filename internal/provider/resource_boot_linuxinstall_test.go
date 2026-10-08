// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func newTestBootLinuxServer() *httptest.Server {
	active := false
	mux := http.NewServeMux()

	mux.HandleFunc("/boot/123/linux", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			active = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"linux": map[string]interface{}{
					"server_ip":       "1.2.3.4",
					"server_ipv6_net": "2a01:4f8::/64",
					"server_number":   123,
					"dist":            "Ubuntu 22.04",
					"lang":            "en",
					"active":          true,
					"password":        "linux-pass-123",
				},
			})
		case http.MethodGet:
			if active {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"linux": map[string]interface{}{
						"server_ip":       "1.2.3.4",
						"server_ipv6_net": "2a01:4f8::/64",
						"server_number":   123,
						"dist":            "Ubuntu 22.04",
						"lang":            "en",
						"active":          true,
						"password":        nil,
					},
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"linux": map[string]interface{}{
						"server_ip":       "1.2.3.4",
						"server_ipv6_net": "2a01:4f8::/64",
						"server_number":   123,
						"dist":            []string{"Ubuntu 22.04", "Debian 12"},
						"lang":            []string{"en", "de"},
						"active":          false,
						"password":        nil,
					},
				})
			}
		case http.MethodDelete:
			active = false
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"linux": map[string]interface{}{
					"server_ip":       "1.2.3.4",
					"server_ipv6_net": "2a01:4f8::/64",
					"server_number":   123,
					"dist":            []string{"Ubuntu 22.04", "Debian 12"},
					"lang":            []string{"en", "de"},
					"active":          false,
					"password":        nil,
				},
			})
		}
	})

	return httptest.NewServer(mux)
}

func TestUnitBootLinuxResource_Create(t *testing.T) {
	ts := newTestBootLinuxServer()
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: `
resource "hetzner_boot_linux" "test" {
  server_number = 123
  dist          = "Ubuntu 22.04"
  lang          = "en"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("hetzner_boot_linux.test", "server_number", "123"),
					resource.TestCheckResourceAttr("hetzner_boot_linux.test", "dist", "Ubuntu 22.04"),
					resource.TestCheckResourceAttr("hetzner_boot_linux.test", "lang", "en"),
					resource.TestCheckResourceAttr("hetzner_boot_linux.test", "active", "true"),
					resource.TestCheckResourceAttr("hetzner_boot_linux.test", "server_ip", "1.2.3.4"),
				),
			},
		},
	})
}

func TestUnitBootLinuxDataSource(t *testing.T) {
	ts := newTestBootLinuxServer()
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: `
data "hetzner_boot_linux" "test" {
  server_number = 123
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.hetzner_boot_linux.test", "server_number", "123"),
					resource.TestCheckResourceAttr("data.hetzner_boot_linux.test", "server_ip", "1.2.3.4"),
					resource.TestCheckResourceAttr("data.hetzner_boot_linux.test", "active", "false"),
				),
			},
		},
	})
}

func TestUnitBootLinuxResource_AuthorizedKeys(t *testing.T) {
	var sent [][]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			sent = append(sent, r.PostForm["authorized_key[]"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"linux": map[string]interface{}{
				"server_ip": "1.2.3.4", "server_number": 123, "dist": "Debian 13 base", "lang": "en", "active": true,
			},
		})
	}))
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: `
resource "hetzner_boot_linux" "test" {
  server_number   = 123
  dist            = "Debian 13 base"
  lang            = "en"
  authorized_key  = "aa:bb"
  authorized_keys = ["cc:dd"]
}`,
				ExpectError: regexp.MustCompile(`Set either authorized_key or authorized_keys`),
			},
			{
				Config: `
resource "hetzner_boot_linux" "test" {
  server_number   = 123
  dist            = "Debian 13 base"
  lang            = "en"
  authorized_keys = ["aa:bb", "cc:dd"]
}`,
				Check: func(_ *terraform.State) error {
					if len(sent) != 1 || strings.Join(sent[0], ",") != "aa:bb,cc:dd" {
						return fmt.Errorf("authorized_key[] sent = %v, want one POST with [aa:bb cc:dd]", sent)
					}
					return nil
				},
			},
		},
	})
}
