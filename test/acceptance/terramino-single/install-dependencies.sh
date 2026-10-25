#!/bin/bash
# Install Docker and clone Terramino — adapted from
# hashicorp-education/learn-vagrant-get-started install-dependencies.sh
set -euo pipefail

apt-get update
apt-get install -y ca-certificates curl gnupg git

# Docker GPG key and repo
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
  https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  | tee /etc/apt/sources.list.d/docker.list > /dev/null

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

# Reload helper
cat > /usr/local/bin/reload-terramino << 'SCRIPT'
#!/bin/bash
cd /home/vagrant/terramino-go
docker compose down
docker compose build --no-cache
docker compose up -d
SCRIPT
chmod +x /usr/local/bin/reload-terramino
