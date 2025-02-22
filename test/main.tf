terraform {
  required_providers {
    setup = {
      source = "lugolven/setup"
    }
    http = {
      source = "hashicorp/http"
      version = "3.4.5"
    }
  }
}

provider "setup" {
  private_key = "../.ssh/id_rsa"
  user        = "test"
  host        = "localhost"
  port        = "1234"
}

resource "setup_user" "test" {
  name   = "test-user"
  groups = [setup_group.test.gid]
}

resource "setup_group" "somegroup" {
  name = "some-group"
}

resource "setup_group" "test" {
  name = "test-group"
}

resource "setup_directory" "test" {
  path  = "/tmp/test"
  owner = setup_user.test.uid
  group = setup_group.test.gid
  mode  = "0755"
}
resource "setup_file" "test" {
  path = "/tmp/test.txt"
  owner = setup_user.test.uid
  group = setup_group.test.gid
  mode = "0644"
  content = "Hello, World!"
  depends_on = [ setup_directory.test ]
}


data "http" "docker_gpg" {
  url = "https://download.docker.com/linux/ubuntu/gpg"
}


resource "setup_apt_repository" "docker" {
  url = "https://download.docker.com/linux/ubuntu"
  key = data.http.docker_gpg.response_body
  name = "docker"
}

resource "setup_apt_packages" "packages" {
  package {
    name = "docker-ce"
  }

  package {
    name = "vlc"
    absent = true
  }

  package {
    name = "firefox"
    absent = true
  }

  depends_on = [ setup_apt_repository.docker ]
}