package provider

import "fmt"

func testProviderConfig(privateKey string, user string, host string, port string) string {
	return fmt.Sprintf(`
	provider "setup" {
		private_key = "%s"
		user        = "%s"
		host        = "%s"
		port        = "%s"
	}
		`, privateKey, user, host, port)
}
