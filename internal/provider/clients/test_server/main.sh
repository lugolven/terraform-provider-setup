#! /bin/bash

echo "Writing authorized_keys"
mkdir -p /home/test/.ssh
echo $1 > /home/test/.ssh/authorized_keys

echo "Setting up docker group permissions"
# Create docker group if it doesn't exist
groupadd docker 2>/dev/null || true
# Add test user to docker group
usermod -aG docker test

# Ensure docker socket has proper permissions
if [ -S /var/run/docker.sock ]; then
    # Get the actual group ID of the docker socket from the host
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)
    echo "Docker socket group ID: $DOCKER_GID"
    
    # Update the docker group to match the host's docker group ID
    groupmod -g $DOCKER_GID docker 2>/dev/null || true
    
    # Ensure the socket is owned by the docker group
    chown root:docker /var/run/docker.sock
    chmod 660 /var/run/docker.sock
    
    # Verify permissions
    ls -la /var/run/docker.sock
fi

# Verify test user is in docker group
echo "Verifying test user group membership:"
groups test

exec /usr/sbin/sshd -D -e