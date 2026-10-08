# Activates a Linux installation on next boot.
resource "hetzner_boot_linux" "example" {
  server_number = 12345
  dist          = "Rescue System"
  lang          = "en"
}

# Authorizes several Robot SSH keys for root on the installed system.
resource "hetzner_boot_linux" "with_keys" {
  server_number   = 12345
  dist            = "Debian 13 base"
  lang            = "en"
  authorized_keys = [hetzner_ssh_key.ops.fingerprint, hetzner_ssh_key.automation.fingerprint]
}
