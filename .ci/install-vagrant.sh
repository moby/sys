#!/bin/bash
set -eux -o pipefail
VAGRANT_VERSION="2.2.10"

# Based on code from https://github.com/opencontainers/runc
DEB="vagrant_${VAGRANT_VERSION}_$(uname -m).deb"
wget "https://releases.hashicorp.com/vagrant/${VAGRANT_VERSION}/$DEB"
apt-get update
apt-get install -q -y \
	bridge-utils \
	dnsmasq-base \
	ebtables \
	libvirt-bin \
	libvirt-dev \
	qemu-kvm \
	qemu-utils \
	ruby-dev \
	./"$DEB"
rm -f "$DEB"
vagrant plugin install vagrant-libvirt
