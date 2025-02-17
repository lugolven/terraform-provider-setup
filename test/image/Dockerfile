FROM debian:latest


# Install SSH server
RUN apt-get update && \
    apt-get install -y ssh lsof && \
    apt-get clean

RUN useradd -ms /bin/bash test

RUN echo "test:pass" | chpasswd

# Enable password authentication
RUN sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config

RUN mkdir /run/sshd

RUN apt install sudo -y
RUN echo "test ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers
COPY ./authorized_keys /home/test/.ssh/authorized_keys

EXPOSE 22

RUN apt-get update && \
    apt-get install -y vlc

# Start SSH service
CMD ["/usr/sbin/sshd", "-D", "-e"]