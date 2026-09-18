#!/usr/bin/env bash
# One-shot server setup for a fresh Hostinger amd64 box, Ubuntu 24.04 with the
# Docker template. Run once as root over SSH; everything after this is
# `./deploy/up.sh`.
#
#   scp deploy/bootstrap.sh root@<ip>:/tmp/ && ssh root@<ip> 'bash /tmp/bootstrap.sh'
#
# Same script for the production box and the staging one — they differ in .env
# and in nothing else (INFRA.md §8 "Staging").
#
# Not Ansible: two hosts, one run each, no inventory and no drift to converge. A
# playbook here is a dependency and a second language for forty lines of apt.
set -euo pipefail

APP_USER=devforge
APP_DIR=/opt/devforge

[ "$(id -u)" = 0 ] || { echo "run as root"; exit 1; }

echo "==> packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq ca-certificates curl git ufw fail2ban unattended-upgrades cron

echo "==> docker engine"
# The Hostinger template ships Docker, so this is a check rather than an install
# (INFRA.md §9.1). Fail loudly instead of installing a second copy from another
# repository: a box running two Docker installs is a box where `docker ps` and
# the compose file disagree about which daemon holds the containers.
if ! command -v docker >/dev/null || ! docker compose version >/dev/null 2>&1; then
  echo "no docker, or no compose plugin — pick the Ubuntu 24.04 + Docker template" >&2
  echo "when creating the VPS, or install Docker by hand before running this" >&2
  exit 1
fi
docker --version

# Lab containers are the point of the product and they are cattle: without a
# cap, one runaway lab's log fills 200 GB and takes the database down with it.
cat > /etc/docker/daemon.json <<'JSON'
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "10m", "max-file": "3" }
}
JSON
systemctl restart docker

echo "==> app user"
id -u "$APP_USER" >/dev/null 2>&1 || useradd -m -s /bin/bash "$APP_USER"
usermod -aG docker "$APP_USER"
mkdir -p "$APP_DIR"
chown "$APP_USER:$APP_USER" "$APP_DIR"

# The deploy key gets in as this user and nothing else. Copy whatever key you
# used to reach root here too, or CD has no way in.
install -d -m 700 -o "$APP_USER" -g "$APP_USER" "/home/$APP_USER/.ssh"
if [ -f /root/.ssh/authorized_keys ]; then
  cp /root/.ssh/authorized_keys "/home/$APP_USER/.ssh/authorized_keys"
  chown "$APP_USER:$APP_USER" "/home/$APP_USER/.ssh/authorized_keys"
  chmod 600 "/home/$APP_USER/.ssh/authorized_keys"
fi

echo "==> firewall"
ufw --force reset >/dev/null
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable
# ⚠️ Hostinger has its own firewall in hPanel, and ufw is the SECOND door, not
# the first: 80 and 443 have to be opened there as well (INFRA.md §9.1). Forget
# that one and the box stays unreachable while `ufw status` reports everything
# is fine — which is why this note is here rather than only in the runbook.

echo "==> ssh hardening"
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
systemctl reload ssh || systemctl reload sshd

echo "==> swap"
# 2 GB, down from the 4 GB the Oracle box needed. That size was sized for the Go
# and Vite builds, and nothing is built here any more (INFRA.md §9.2) — images
# are pulled. What is left is headroom for a burst of lab containers, so the
# reaper gets a chance to run instead of the kernel picking a victim.
if ! swapon --show | grep -q .; then
  fallocate -l 2G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile >/dev/null
  swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

echo "==> nightly backup cron"
# This used to be a comment at the top of scripts/backup.sh telling a human to
# install it. Following the setup exactly and still ending up with no backup at
# all is not a setup step, so it happens here instead. The script itself no-ops
# loudly until R2_* are filled in.
#
# The log file is created up front and owned by APP_USER: /var/log is root's,
# and cron running as devforge cannot create the file it appends to.
touch /var/log/devforge-backup.log
chown "$APP_USER:$APP_USER" /var/log/devforge-backup.log
chmod 640 /var/log/devforge-backup.log
cat > /etc/cron.d/devforge-backup <<CRON
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
15 3 * * * $APP_USER cd $APP_DIR/devforge-be && ./scripts/backup.sh >> /var/log/devforge-backup.log 2>&1
CRON
chmod 644 /etc/cron.d/devforge-backup
systemctl enable --now cron

cat > /etc/logrotate.d/devforge-backup <<'ROTATE'
/var/log/devforge-backup.log {
  weekly
  rotate 8
  compress
  missingok
  notifempty
  copytruncate
}
ROTATE

echo "==> unattended security upgrades"
dpkg-reconfigure -f noninteractive unattended-upgrades

cat <<TXT

done.

next, as $APP_USER — only the be repo is cloned, the FE ships as an image now:
  git clone <be repo> $APP_DIR/devforge-be
  cd $APP_DIR/devforge-be
  cp .env.prod.example .env && chmod 600 .env   # then fill it in
  install -m 600 /dev/null deploy/nginx/certs/origin.key   # paste the key in
  install -m 644 /dev/null deploy/nginx/certs/origin.pem   # paste the cert in
  IMAGE_TAG=<tag> ./deploy/up.sh

there is no ACME here: TLS is a Cloudflare Origin Certificate, two files you
paste onto the box. Nothing expires for fifteen years and nothing is requested
at startup, so the DNS record does not have to exist before the first up — but
the orange cloud does have to be ON afterwards, because that certificate is
trusted by Cloudflare and by no browser. Full runbook: deploy/DEPLOY.md.
TXT
