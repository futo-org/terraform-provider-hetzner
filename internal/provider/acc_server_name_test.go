// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const testAccServerNameSuffix = "-tfacc"

// TestAccServerName_Rename renames the persistent test server and restores its
// name. The suffix is stripped from the starting name so an interrupted run
// cannot stack suffixes on the shared server.
func TestAccServerName_Rename(t *testing.T) {
	serverNumber := testAccGetOrCreateServer(t)
	original := strings.TrimSuffix(testAccRobotServerName(t, serverNumber), testAccServerNameSuffix)
	if original == "" {
		t.Skipf("server %s has no name to restore", serverNumber)
	}
	renamed := original + testAccServerNameSuffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRobotServerName(t, serverNumber, original),
		Steps: []resource.TestStep{
			{
				Config: testAccServerNameConfig(serverNumber, renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("hetzner_server_name.test", "server_name", renamed),
					testAccCheckRobotServerName(t, serverNumber, renamed),
				),
			},
			{
				ResourceName:                         "hetzner_server_name.test",
				ImportState:                          true,
				ImportStateId:                        serverNumber,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "server_number",
			},
			{
				Config: testAccServerNameConfig(serverNumber, original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("hetzner_server_name.test", "server_name", original),
					testAccCheckRobotServerName(t, serverNumber, original),
				),
			},
		},
	})
}

func testAccServerNameConfig(serverNumber, name string) string {
	return fmt.Sprintf(`
resource "hetzner_server_name" "test" {
  server_number = %s
  server_name   = %q
}`, serverNumber, name)
}

func testAccRobotServerName(t *testing.T, serverNumber string) string {
	t.Helper()
	body, err := testAccNewClient(t).Get("/server/" + serverNumber)
	if err != nil {
		t.Fatalf("reading server %s: %v", serverNumber, err)
	}
	var resp serverDetailAPIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("parsing server %s: %v", serverNumber, err)
	}
	return resp.Server.ServerName
}

// testAccCheckRobotServerName reads Robot directly rather than state, so it
// also proves a destroy left the server's name alone.
func testAccCheckRobotServerName(t *testing.T, serverNumber, want string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if got := testAccRobotServerName(t, serverNumber); got != want {
			return fmt.Errorf("Robot name of server %s = %q, want %q", serverNumber, got, want)
		}
		return nil
	}
}
