A simple Terraform provider to set up bare-metal machines.

## Provider configuration

| Name | Required | Description |
| --- | --- | --- |
| `user` | yes | User to use for SSH authentication. |
| `host` | yes | Host to connect to. |
| `port` | yes | Port to connect to. |
| `private_key` | no | Path to the private key to use for SSH authentication. |
| `ssh_agent` | no | Path to the SSH agent socket. |
| `max_concurrent_sessions` | no | Maximum number of SSH sessions opened concurrently against the target host. Defaults to `5`. |

### `max_concurrent_sessions`

Every resource operation opens an SSH session against the target host. Terraform
(and OpenTofu) refresh and apply resources concurrently at `-parallelism`
(default 10, and consumers may set it higher). When the number of
simultaneously-open sessions exceeds the host sshd's `MaxSessions` (default 10),
sshd rejects the extra session-channel opens and the provider surfaces:

```
ssh: rejected: connect failed (open failed)
```

To defend against this regardless of the caller's `-parallelism`, the provider
caps concurrent SSH sessions internally with a semaphore. The limit is
configured with `max_concurrent_sessions`:

- Defaults to `5`, comfortably under sshd's default `MaxSessions` of 10.
- Can also be set via the `SETUP_MAX_CONCURRENT_SESSIONS` environment variable
  (the `max_concurrent_sessions` attribute takes precedence when both are set).
- Set to `0` to disable the limit (unbounded — the historical behavior).
- Negative values are rejected.

If you raise `MaxSessions` on the host, you can raise this attribute to match.

```hcl
provider "setup" {
  user = "root"
  host = "192.0.2.10"
  port = "22"

  # Optional; defaults to 5.
  max_concurrent_sessions = 5
}
```
