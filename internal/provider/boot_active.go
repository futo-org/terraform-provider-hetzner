// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"

	"github.com/zack/terraform-provider-hetzner/internal/client"
)

// bootActive reports whether a boot configuration ("rescue", "linux", ...) is
// still armed. Robot clears the activation when the server boots into it, so
// an inactive one means the boot already happened.
func bootActive(c *client.Client, serverNum int64, kind string) (bool, error) {
	body, err := c.Get(fmt.Sprintf("/boot/%d/%s", serverNum, kind))
	if err != nil {
		return false, err
	}
	var apiResp map[string]struct {
		Active bool `json:"active"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return false, err
	}
	return apiResp[kind].Active, nil
}
