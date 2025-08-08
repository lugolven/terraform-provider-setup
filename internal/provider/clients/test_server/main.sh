#! /bin/bash

echo "Writing authorized_keys"
mkdir -p /home/test/.ssh
echo $1 > /home/test/.ssh/authorized_keys

echo "Setting up docker group permissions"
# Create docker group if it doesn't exist
groupadd docker 2>/dev/null || true
# Add test user to docker group
usermod -aG docker test

exec /usr/sbin/sshd -D -e