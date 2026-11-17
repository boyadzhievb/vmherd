#!/bin/bash
# Install Docker, avahi, and clone Terramino — adapted from
# hashicorp-education/learn-vagrant-get-started common-dependencies.sh
set -euo pipefail

apt-get update
apt-get install -y ca-certificates curl gnupg git avahi-daemon libnss-mdns

# Docker GPG key and repo
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  | gpg --batch --yes --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
  https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  | tee /etc/apt/sources.list.d/docker.list > /dev/null

# Use the classic overlay2 image store, written before Docker is installed so
# the daemon starts with it. Docker 29's default containerd image store fails
# to extract a layer of golang:1.22, the base image of Terramino's
# Dockerfile.backend ("archive/tar: invalid tar header").
install -m 0755 -d /etc/docker
echo '{"features":{"containerd-snapshotter":false}}' > /etc/docker/daemon.json

apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io \
  docker-buildx-plugin docker-compose-plugin

usermod -aG docker vagrant

# Clone Terramino into the shared folder.
# The directory is a 9p mount, so we use git init + fetch instead of clone.
# Every git command uses -c safe.directory= because cloud-init runs as root
# but the 9p mount shows a different UID (dubious ownership check).
# We check for docker-compose.yml rather than .git because a stale .git may
# persist on the host shared folder across VM recreations.
REPO=/home/vagrant/terramino-go
SAFE="-c safe.directory=${REPO}"
if [ ! -f "${REPO}/docker-compose.yml" ]; then
  cd "${REPO}"
  git ${SAFE} init
  git ${SAFE} remote add origin https://github.com/hashicorp-education/terramino-go.git || true
  git ${SAFE} fetch origin
  git ${SAFE} checkout -B containerized origin/containerized
fi
