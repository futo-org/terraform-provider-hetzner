# Activates the rescue system on next boot.
resource "hetzner_boot_rescue" "example" {
  server_number = 12345
  os            = "linux"
}

# Authorizes several Robot SSH keys for root in the rescue system.
resource "hetzner_boot_rescue" "with_keys" {
  server_number   = 12345
  os              = "linux"
  authorized_keys = [hetzner_ssh_key.ops.fingerprint, hetzner_ssh_key.automation.fingerprint]
}
