package provider

import (
	"context"
	"fmt"
	"terraform-provider-setup/internal/provider/clients"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAptRepositoryResource(t *testing.T) {
	// Docker GPG key for testing
	const dockerGpgKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBFit2ioBEADhWpZ8/wvZ6hUTiXOwQHXMAlaFHcPH9hAtr4F1y2+OYdbtMuth
lqqwp028AqyY+PRfVMtSYMbjuQuu5byyKR01BbqYhuS3jtqQmljZ/bJvXqnmiVXh
38UuLa+z077PxyxQhu5BbqntTPQMfiyqEiU+BEasoYO3rSI0lFz/Z9A1/kfgfJGe
B8qpZWDY4w7Z6f8ZdLWJTF3p0OgYrhSkGk8bqmOKRhVJT6Y2Wlz2aXWxMqVcWpA9
MYqOOqNL8r8VDqt7Y6iYYzHhq0Vh2eHNlQzjKd1NwgYaXOhfOIlc1JG5rK0l7ZOA
zY8xSx44HqUADhO6nKP4eY1M2iEFKZFKKn4KgIKnW4nMCq4/nLpH0m8qFE5+fRtk
ZW0A9jJj8xIQzKNnVKGXdhL2dC9WqVCYZGBzWXtGG9oVJl4VhSPL1bgS5LMRlQT5
+iXQFJ6zPGSLRYO9o5cC1DQ2UY8I8xqbsNnDbNIIFQv1VVG1l/9nq6Qps9Oah3VM
8qNEPDbBQSMNgYCjPPOKYgCKZDhOCgKCo9Vh0eEvP0qFtZP7TQa9mFXIUAyj6n0X
MMZ/d+eFH1Q8VhPO9lPK0oVp8x/qKLI8Q6iHqhITd3KxH+vw+c0v6KcnTgzz9D5d
sJFDcVbvqYPYG7rNfVd8y6ZCx3AiQ2dE7rKxg1FKnH4T5JJKjK+jcU8eKQARAQAB
tCtEb2NrZXIgUmVsZWFzZSAoQ0UgZGViKSA8ZG9ja2VyQGRvY2tlci5jb20+iQI3
BBMBCgAhBQJYrdoqAhsDBQsJCAcDBRUKCQgLBRYCAwEAAh4BAheAAAoJEI2BgDwO
v82IsskP/iQZo68flDQmNvn8X5XTd6RRaUH33kXYXquT6NdLUK7nGX5Xx1OOCqLN
bAmJVX4lEe7HXQB/X9Z7EcGl6ZaJRdPOzm8z3ysJ8tJJo5YJe6KfxzPQAeDOmNKz
E4THk/VlKRZJLSkUu3XPDZ6NZZZJe6FaYjJlO5QMpQMzMLjz5yPGM3DlBDFLc/eS
Q2VF4EWd9ZLjJdO7bFdKELBZNlIJb3vOJeBKFgqCw+WdO7m/A1K2EpqMkIkxP0Yf
VkhCJz9yGT4KGJqPfZJqKOy9JGJdTWxQMwNZtJ6tJqN4/EKcK+o4VQgGK9OC5cYr
VoNzAHHJXp5QlZYOWoNZKHKd7OJqTJqX8+o8WJq7V3TkZh2F3QnC6JYNdOhiP3qr
VQ2mZR6RYqNGJX4KCJ8w4nMc8pQOb5QiJ5GCdFZJXdNJTy9q1rZt8+sFYaHJ5VFK
eOmJJVIm8Y7P6V0V5IaJqcM1jY6Ee5+fJz3v6T7r5YMnQkfX1lc0lIfIMLRf0Cj2
ePXkQ3OdJxlQqOBJrJLKn5U6J7vq5M6vJdJmUMzj+z7Y4GXyJjJKrpJbFUTL2lz+
s9fhYQNYeP8nKlNKYgQGGMfwJzOJl4nHgMWX8V/9e0qKfgjkYoA/vxYVFZ6bY/oY
LVJ6aPO4EkqZ5qUY/xj7kV8f4gJ0VJVa7wQQJKy4VJ9qSj3j7+cK
=QAVV
-----END PGP PUBLIC KEY BLOCK-----`

	t.Run("Test create, update and delete", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfig("docker", dockerGpgKey, "https://download.docker.com/linux/ubuntu"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "docker"),
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "key", dockerGpgKey+"\n"),
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "url", "https://download.docker.com/linux/ubuntu"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check if key file exists
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/docker.asc")
							if err != nil {
								return fmt.Errorf("key file not found: %w", err)
							}

							// Check if source list exists
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/docker.list")
							if err != nil {
								return fmt.Errorf("source list not found: %w", err)
							}

							// Verify content of source list
							content, err := sshClient.RunCommand(context.Background(), "cat /etc/apt/sources.list.d/docker.list")
							if err != nil {
								return fmt.Errorf("failed to read source list: %w", err)
							}

							if content == "" {
								return fmt.Errorf("source list is empty")
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfig("docker", dockerGpgKey, "https://download.docker.com/linux/debian"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "docker"),
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "key", dockerGpgKey+"\n"),
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "url", "https://download.docker.com/linux/debian"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Verify updated content of source list
							content, err := sshClient.RunCommand(context.Background(), "cat /etc/apt/sources.list.d/docker.list")
							if err != nil {
								return fmt.Errorf("failed to read source list: %w", err)
							}

							if content == "" {
								return fmt.Errorf("source list is empty")
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost"),
					Check: resource.ComposeTestCheckFunc(
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that key file was deleted
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/docker.asc")
							if err == nil {
								return fmt.Errorf("key file should have been deleted")
							}

							// Check that source list was deleted
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/docker.list")
							if err == nil {
								return fmt.Errorf("source list should have been deleted")
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test name change", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfig("old-repo", dockerGpgKey, "https://download.docker.com/linux/ubuntu"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "old-repo"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check if old files exist
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/old-repo.asc")
							if err != nil {
								return fmt.Errorf("old key file not found: %w", err)
							}

							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/old-repo.list")
							if err != nil {
								return fmt.Errorf("old source list not found: %w", err)
							}

							return nil
						},
					),
				},
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfig("new-repo", dockerGpgKey, "https://download.docker.com/linux/ubuntu"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "new-repo"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that old files were removed
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/old-repo.asc")
							if err == nil {
								return fmt.Errorf("old key file should have been removed")
							}

							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/old-repo.list")
							if err == nil {
								return fmt.Errorf("old source list should have been removed")
							}

							// Check that new files exist
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/new-repo.asc")
							if err != nil {
								return fmt.Errorf("new key file not found: %w", err)
							}

							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/sources.list.d/new-repo.list")
							if err != nil {
								return fmt.Errorf("new source list not found: %w", err)
							}

							return nil
						},
					),
				},
			},
		})
	})

	t.Run("Test read with missing files", func(t *testing.T) {
		// Arrange
		setup := setupTestEnvironment(t)

		// Act & assert
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: getTestProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfig("test-repo", dockerGpgKey, "https://download.docker.com/linux/ubuntu"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "test-repo"),
					),
				},
				{
					PreConfig: func() {
						sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
						if err != nil {
							t.Fatal(err)
						}

						// Manually remove key file to simulate external change
						_, err = sshClient.RunCommand(context.Background(), "sudo rm -f /etc/apt/keyrings/test-repo.asc")
						if err != nil {
							t.Fatalf("failed to remove key file: %v", err)
						}
					},
					Config: testProviderConfig(setup, "test", "localhost") + testAptRepositoryResourceConfig("test-repo", dockerGpgKey, "https://download.docker.com/linux/ubuntu"),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("setup_apt_repository.repo", "name", "test-repo"),
						func(_ *terraform.State) error {
							sshClient, err := clients.CreateSSHMachineAccessClientBuilder("test", "localhost", setup.Port).WithPrivateKeyPath(setup.KeyPath).Build(context.Background())
							if err != nil {
								return err
							}

							// Check that key file was recreated
							_, err = sshClient.RunCommand(context.Background(), "test -f /etc/apt/keyrings/test-repo.asc")
							if err != nil {
								return fmt.Errorf("key file should have been recreated: %w", err)
							}

							return nil
						},
					),
				},
			},
		})
	})
}

func testAptRepositoryResourceConfig(name string, key string, url string) string {
	return fmt.Sprintf(`
resource "setup_apt_repository" "repo" {
	name = "%s"
	key  = <<EOT
%s
EOT
	url  = "%s"
}
`, name, key, url)
}