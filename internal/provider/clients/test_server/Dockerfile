
# todo: test this with other images such as debian
FROM cruizba/ubuntu-dind:latest

# Install SSH server
RUN apt-get update && \
    apt-get install -y ssh lsof && \
    apt-get clean

RUN useradd -ms /bin/bash test

RUN echo "test:pass" | chpasswd
RUN groupadd docker
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