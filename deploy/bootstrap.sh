#!/usr/bin/env bash
# One-shot server setup for a fresh Ubuntu 22.04/24.04 ARM64 box. Run once as
# root over SSH; everything after this is `git pull && compose up`.
#
#   scp deploy/bootstrap.sh ubuntu@<ip>:/tmp/ && ssh ubuntu@<ip> 'sudo bash /tmp/bootstrap.sh'
#
# Not Ansible: one host, one run, no inventory and no drift to converge. A
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
if ! command -v docker >/dev/null; then
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
  chmod a+r /etc/apt/keyrings/docker.asc
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
fi

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
# Oracle Cloud also has its own security list in the VCN. ufw is the second
# door, not the first: 80 and 443 have to be opened there as well or the box
# stays unreachable no matter what ufw says.

echo "==> ssh hardening"
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
systemctl reload ssh || systemctl reload sshd

echo "==> swap"
# 24 GB of RAM does not need swap to run, but the Go and Vite builds run on
# this box now (INFRA.md §9.2) and a build OOM-killing Postgres is a worse
# outcome than a slow build.
if ! swapon --show | grep -q .; then
  fallocate -l 4G /swapfile
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

next, as $APP_USER:
  git clone <be repo> $APP_DIR/devforge-be
  git clone <fe repo> $APP_DIR/devforge-fe
  cd $APP_DIR/devforge-be
  cp .env.prod.example .env && chmod 600 .env   # then fill it in
  ./deploy/up.sh

then point the domain's A record at this box BEFORE the first up: Caddy asks
Let's Encrypt over HTTP-01 and a failed challenge retries into a rate limit.
TXT
