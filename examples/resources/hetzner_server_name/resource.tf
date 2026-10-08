# Sets the Robot name of an existing server. Destroying it leaves the server untouched.
resource "hetzner_server_name" "example" {
  server_number = 12345
  server_name   = "my-server"
}
