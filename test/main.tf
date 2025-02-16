terraform {
  required_providers {
    setup = {
      source = "setup"
    }
  }
}

provider "setup" {
  private_key = "../.ssh/id_rsa"
  user = "test"
  host = "localhost"
  port = "1234"
}

resource "setup_user" "test" {
  name = "test-user"
  groups = [setup_group.test.gid]
}

resource "setup_group" "somegroup" {
  name = "some-group" 
}

resource "setup_group" "test" {
  name = "test-group"
}

resource "setup_directory" "test" {
  path = "/tmp/test"
  owner = setup_user.test.uid
  group = setup_group.test.gid
  mode = "0755"
}