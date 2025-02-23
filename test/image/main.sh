#! /bin/bash

echo "Writing authorized_keys"
mkdir -p /home/test/.ssh
echo $1 > /home/test/.ssh/authorized_keys

exec /usr/sbin/sshd -D -e