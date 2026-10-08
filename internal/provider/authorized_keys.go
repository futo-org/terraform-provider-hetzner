// Copyright (c) Zack
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func checkAuthorizedKeysConflict(single types.String, list types.List, diags *diag.Diagnostics) {
	if !single.IsNull() && !list.IsNull() {
		diags.AddAttributeError(frameworkPath("authorized_keys"), "Conflicting attributes",
			"Set either authorized_key or authorized_keys, not both.")
	}
}

func addAuthorizedKeys(ctx context.Context, data url.Values, single types.String, list types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if !single.IsNull() && !single.IsUnknown() {
		data.Set("authorized_key", single.ValueString())
	}
	if !list.IsNull() && !list.IsUnknown() {
		var keys []string
		diags.Append(list.ElementsAs(ctx, &keys, false)...)
		for _, k := range keys {
			data.Add("authorized_key[]", k)
		}
	}
	return diags
}
