#!/usr/bin/env bash
# One-shot server setup for a fresh Linode (Akamai) amd64 box rebuilt from the
# plain Ubuntu 24.04 LTS image. Run once as root over SSH; everything after this
# is `./deploy/up.sh`.
#
#   scp deploy/bootstrap.sh root@<ip>:/tmp/ && ssh root@<ip> 'bash /tmp/bootstrap.sh'
#
# Same script for the production box and the dev one — they differ in .env
# and in nothing else (INFRA.md §8 "Dev environment").
#
# Not Ansible: two hosts, one run each, no inventory and no drift to converge. A
# playbook here is a dependency and a second language for forty lines of apt.
set -euo pipefail

APP_USER=devforge
APP_DIR=/opt/devforge

[ "$(id -u)" = 0 ] || { echo "run as root"; exit 1; }

# Everything below is apt, dpkg and Ubuntu's service names. On anything else it
# would fail halfway through and leave a half-configured box, so refuse up front.
. /etc/os-release
[ "${ID:-}" = ubuntu ] || { echo "Ubuntu only (found ${PRETTY_NAME:-unknown}) — rebuild the Linode from the Ubuntu 24.04 LTS image" >&2; exit 1; }

echo "==> packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq ca-certificates curl git ufw fail2ban unattended-upgrades cron

echo "==> docker engine"
# Linode's Ubuntu image ships without Docker, so install it from Docker's own apt
# repository (INFRA.md §9.1) — not Ubuntu's docker.io, which lags and has no
# compose plugin. An existing install is kept, but only if it has the compose
# plugin: a box running two Docker installs is a box where `docker ps` and the
# compose file disagree about which daemon holds the containers.
if ! command -v docker >/dev/null; then
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
  chmod a+r /etc/apt/keyrings/docker.asc
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${UBUNTU_CODENAME:-$VERSION_CODENAME} stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  systemctl enable --now docker
fi
docker compose version >/dev/null 2>&1 || {
  echo "docker is installed but has no compose plugin — remove that install, then re-run" >&2
  exit 1
}
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
# ⚠️ If a Linode Cloud Firewall is attached to this box, it is the FIRST door and
# ufw the second: 22, 80 and 443 have to be open there as well (INFRA.md §9.1).
# Forget that one and the box stays unreachable while `ufw status` reports
# everything is fine — which is why this note is here rather than only in the
# runbook.

echo "==> ssh hardening"
# A drop-in, not sed on sshd_config: Ubuntu's sshd_config Includes
# sshd_config.d/*.conf at the top and the first value for a keyword wins, so a
# cloud-init drop-in saying `PasswordAuthentication yes` would silently beat an
# edit further down. 00- sorts first. Validated before the reload so a typo
# cannot lock the box out.
cat > /etc/ssh/sshd_config.d/00-devforge.conf <<'SSHD'
PermitRootLogin prohibit-password
PasswordAuthentication no
KbdInteractiveAuthentication no
SSHD
sshd -t
systemctl reload ssh || systemctl reload sshd
sshd -T | grep -E '^(permitrootlogin|passwordauthentication) '

echo "==> swap"
# At least 2 GB. Nothing is built here (INFRA.md §9.2) — images are pulled — so
# this is headroom for a burst of lab containers, letting the reaper run before
# the kernel picks a victim. Linode's image already has a ~512 MB swap disk,
# which is why this tops up below 2 GB rather than skipping whenever any swap
# exists.
if [ "$(free -m | awk '/^Swap:/{print $2}')" -lt 2000 ] && [ ! -f /swapfile ]; then
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
trusted by Cloudflare and by no browser. Full runbook: INFRA.md §13.
TXT
