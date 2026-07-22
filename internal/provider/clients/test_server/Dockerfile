
# todo: test this with other images such as debian
# Pinned to a known-good tag instead of :latest. The floating :latest tag
# drifted (currently a 29.6.x rebuild) and broke the acceptance-test image
# build; jammy-29.1.1 is the last release from the window when CI was green.
FROM cruizba/ubuntu-dind:jammy-29.1.1

# Install SSH server
# Remove pre-configured docker apt sources from the base image so tests can
# add their own without triggering "Conflicting values set for Signed-By" errors
RUN rm -f /etc/apt/sources.list.d/docker.list /etc/apt/keyrings/docker.asc

RUN apt-get update && \
    apt-get install -y ssh lsof && \
    apt-get clean

RUN useradd -ms /bin/bash test

RUN echo "test:pass" | chpasswd
RUN groupadd -f docker
RUN usermod -aG docker test 

# Enable password authentication
RUN sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config

RUN mkdir /run/sshd

RUN apt install sudo -y
RUN echo "test ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers
COPY ./main.sh /app/main.sh
EXPOSE 22

# Start SSH service
ENTRYPOINT  ["/app/main.sh"]