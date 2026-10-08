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

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/zack/terraform-provider-hetzner/internal/client"
)

type fakeServerNames struct {
	mu    sync.Mutex
	names map[int]string
}

func (f *fakeServerNames) set(num int, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names[num] = name
}

func (f *fakeServerNames) remove(num int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.names, num)
}

func (f *fakeServerNames) get(num int) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name, ok := f.names[num]
	return name, ok
}

func newTestServerNameServer(t *testing.T, f *fakeServerNames) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/server":
			f.mu.Lock()
			list := []serverListAPIResponse{}
			for num, name := range f.names {
				list = append(list, serverListAPIResponse{Server: serverListAPI{ServerNumber: num, ServerName: name, Status: "ready"}})
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(list)
		case r.Method == http.MethodPost && r.URL.Path == "/server/321":
			if _, ok := f.get(321); !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"status":404,"code":"SERVER_NOT_FOUND","message":"server not found"}}`))
				return
			}
			f.set(321, r.PostFormValue("server_name"))
			name, _ := f.get(321)
			_ = json.NewEncoder(w).Encode(serverListAPIResponse{Server: serverListAPI{ServerNumber: 321, ServerName: name, Status: "ready"}})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
}

// A client per provider start, as in a real run: GetCached must not carry one
// command's server list into the next.
func perRunProviderFactories(ts *httptest.Server) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"hetzner": func() (tfprotov6.ProviderServer, error) {
			c := client.NewClient("test", "test")
			c.BaseURL = ts.URL
			return providerserver.NewProtocol6WithError(newBatch2TestProvider(c)())()
		},
	}
}

func testServerNameConfig(name string) string {
	return fmt.Sprintf(`
resource "hetzner_server_name" "test" {
  server_number = 321
  server_name   = %q
}`, name)
}

func checkFakeServerName(f *fakeServerNames, want string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got, ok := f.get(321)
		if !ok {
			return fmt.Errorf("server 321 missing from the fake Robot")
		}
		if got != want {
			return fmt.Errorf("Robot name for 321 = %q, want %q", got, want)
		}
		return nil
	}
}

func TestUnitServerNameResource_lifecycle(t *testing.T) {
	f := &fakeServerNames{names: map[int]string{321: "old-name", 322: "neighbour"}}
	ts := newTestServerNameServer(t, f)
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: perRunProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: testServerNameConfig("spice-ceph-adelia"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("hetzner_server_name.test", "server_name", "spice-ceph-adelia"),
					checkFakeServerName(f, "spice-ceph-adelia"),
				),
			},
			{
				Config: testServerNameConfig("spice-ceph-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("hetzner_server_name.test", "server_name", "spice-ceph-renamed"),
					checkFakeServerName(f, "spice-ceph-renamed"),
				),
			},
			{
				ResourceName:                         "hetzner_server_name.test",
				ImportState:                          true,
				ImportStateId:                        "321",
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "server_number",
			},
			{
				PreConfig: func() { f.set(321, "renamed-in-robot") },
				Config:    testServerNameConfig("spice-ceph-renamed"),
				Check:     checkFakeServerName(f, "spice-ceph-renamed"),
			},
		},
		CheckDestroy: func(s *terraform.State) error {
			if err := checkFakeServerName(f, "spice-ceph-renamed")(s); err != nil {
				return fmt.Errorf("destroy must leave the server alone: %w", err)
			}
			if name, _ := f.get(322); name != "neighbour" {
				return fmt.Errorf("untracked server 322 changed to %q", name)
			}
			return nil
		},
	})
}

func TestUnitServerNameResource_serverGone(t *testing.T) {
	f := &fakeServerNames{names: map[int]string{321: "spice-ceph-adelia"}}
	ts := newTestServerNameServer(t, f)
	defer ts.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: perRunProviderFactories(ts),
		Steps: []resource.TestStep{
			{
				Config: testServerNameConfig("spice-ceph-adelia"),
			},
			{
				PreConfig:          func() { f.remove(321) },
				Config:             testServerNameConfig("spice-ceph-adelia"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
