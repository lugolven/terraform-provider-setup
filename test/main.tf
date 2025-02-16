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
