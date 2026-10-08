// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func newTestBootRescueServer() *httptest.Server {
	password := "rescue-pass-123"
	active := false
	mux := http.NewServeMux()

	mux.HandleFunc("/boot/123/rescue", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			active = true
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"rescue": map[string]interface{}{
					"server_ip":       "1.2.3.4",
					"server_ipv6_net": "2a01:4f8::/64",
					"server_number":   123,
					"os":              "linux",
					"active":          true,
					"password":        password,
					"keyboard":        "us",
				},
			})
		case http.MethodGet:
			if active {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"rescue": map[string]interface{}{
						"server_ip":       "1.2.3.4",
						"server_ipv6_net": "2a01:4f8::/64",
						"server_number":   123,
						"os":              "linux",
						"active":          true,
						"password":        nil,
						"keyboard":        "us",
					},
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"rescue": map[string]interface{}{
						"server_ip":       "1.2.3.4",
						"server_ipv6_net": "2a01:4f8::/64",
						"server_number":   123,
						"os":              []string{"linux", "vkvm"},
						"active":          false,
						"password":        nil,
						"keyboard":        "us",
					},
				})
			}
		case http.MethodDelete:
			active = false
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"rescue": map[string]interface{}{
					"server_ip":       "1.2.3.4",
					"server_ipv6_net": "2a01:4f8::/64",
					"server_number":   123,
					"os":              []string{"linux", "vkvm"},
					"active":          false,
					"password":        nil,
				},
			})
		}
	})

	return httptest.NewServer(mux)
}

func TestUnitBootRescueResource_Create(t *testing.T) {
	ts := newTestBootRescueServer()
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: `
resource "hetzner_boot_rescue" "test" {
  server_number = 123
  os            = "linux"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("hetzner_boot_rescue.test", "server_number", "123"),
					resource.TestCheckResourceAttr("hetzner_boot_rescue.test", "os", "linux"),
					resource.TestCheckResourceAttr("hetzner_boot_rescue.test", "server_ip", "1.2.3.4"),
					resource.TestCheckResourceAttr("hetzner_boot_rescue.test", "active", "true"),
				),
			},
		},
	})
}

func TestUnitBootRescueResource_Import(t *testing.T) {
	ts := newTestBootRescueServer()
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: `
resource "hetzner_boot_rescue" "test" {
  server_number = 123
  os            = "linux"
}`,
			},
			{
				ResourceName:      "hetzner_boot_rescue.test",
				ImportState:       true,
				ImportStateId:     "123",
				ImportStateVerify: false,
			},
		},
	})
}

func TestUnitBootRescueDataSource(t *testing.T) {
	ts := newTestBootRescueServer()
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: `
data "hetzner_boot_rescue" "test" {
  server_number = 123
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.hetzner_boot_rescue.test", "server_number", "123"),
					resource.TestCheckResourceAttr("data.hetzner_boot_rescue.test", "server_ip", "1.2.3.4"),
					resource.TestCheckResourceAttr("data.hetzner_boot_rescue.test", "active", "false"),
				),
			},
		},
	})
}

func TestUnitBootRescueResource_DestroyAfterBoot(t *testing.T) {
	active := false
	mux := http.NewServeMux()
	mux.HandleFunc("/boot/123/rescue", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete && !active {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"status":409,"code":"CONFLICT","message":"rescue system is not active"}}`))
			return
		}
		switch r.Method {
		case http.MethodPost:
			active = true
		case http.MethodDelete:
			active = false
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"rescue": map[string]interface{}{
				"server_ip":       "1.2.3.4",
				"server_ipv6_net": "2a01:4f8::/64",
				"server_number":   123,
				"os":              "linux",
				"active":          active,
				"password":        nil,
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	config := `
resource "hetzner_boot_rescue" "test" {
  server_number = 123
  os            = "linux"
}`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("hetzner_boot_rescue.test", "active", "true"),
			},
			{
				PreConfig: func() { active = false },
				Config:    config,
				Check:     resource.TestCheckResourceAttr("hetzner_boot_rescue.test", "active", "false"),
			},
		},
	})
}

func TestUnitBootRescueResource_AuthorizedKeys(t *testing.T) {
	var sent [][]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			sent = append(sent, r.PostForm["authorized_key[]"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"rescue": map[string]interface{}{
				"server_ip": "1.2.3.4", "server_number": 123, "os": "linux", "active": true,
			},
		})
	}))
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: `
resource "hetzner_boot_rescue" "test" {
  server_number   = 123
  os              = "linux"
  authorized_key  = "aa:bb"
  authorized_keys = ["cc:dd"]
}`,
				ExpectError: regexp.MustCompile(`Set either authorized_key or authorized_keys`),
			},
			{
				Config: `
resource "hetzner_boot_rescue" "test" {
  server_number   = 123
  os              = "linux"
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

func TestUnitBootRescueResource_AuthorizedKeysUnknownAtPlan(t *testing.T) {
	var sent []url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			sent = append(sent, r.PostForm)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"rescue": map[string]interface{}{
				"server_ip": "1.2.3.4", "server_number": 123, "os": "linux", "active": true,
			},
		})
	}))
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: batch3ProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: `
resource "terraform_data" "single" {
  input = null
}

resource "hetzner_boot_rescue" "test" {
  server_number   = 123
  os              = "linux"
  authorized_key  = terraform_data.single.output
  authorized_keys = ["aa:bb"]
}`,
				Check: func(_ *terraform.State) error {
					if len(sent) != 1 || sent[0].Has("authorized_key") || strings.Join(sent[0]["authorized_key[]"], ",") != "aa:bb" {
						return fmt.Errorf("rescue POSTs = %v, want one with only authorized_key[]=aa:bb", sent)
					}
					return nil
				},
			},
		},
	})
}
