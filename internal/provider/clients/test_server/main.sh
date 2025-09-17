#! /bin/bash

echo "Writing authorized_keys"
mkdir -p /home/test/.ssh
echo $1 > /home/test/.ssh/authorized_keys
/usr/local/bin/entrypoint.sh
chown root:docker /var/run/docker.sock
exec /usr/sbin/sshd -D -e