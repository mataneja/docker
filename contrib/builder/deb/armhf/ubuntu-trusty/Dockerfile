#
# THIS FILE IS AUTOGENERATED; SEE "contrib/builder/deb/armhf/generate.sh"!
#

FROM armhf/ubuntu:trusty

RUN apt-get update && apt-get install -y apparmor bash-completion btrfs-tools build-essential cmake curl ca-certificates debhelper dh-apparmor dh-systemd git libapparmor-dev libdevmapper-dev libltdl-dev  pkg-config vim-common libsystemd-journal-dev --no-install-recommends && rm -rf /var/lib/apt/lists/*

ENV GO_VERSION 1.8.3
RUN curl -fSL "https://golang.org/dl/go${GO_VERSION}.linux-armv6l.tar.gz" | tar xzC /usr/local
ENV PATH $PATH:/usr/local/go/bin

ENV AUTO_GOPATH 1

ENV DOCKER_BUILDTAGS apparmor pkcs11 selinux
ENV RUNC_BUILDTAGS apparmor selinux
