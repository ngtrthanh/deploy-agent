#!/usr/bin/env sh
set -eu

BINARY=${BINARY:-./deploy-agent}
CONFIG=${CONFIG:-./deploy-agent.json}

install -m 0755 "$BINARY" /usr/local/bin/deploy-agent
install -d -m 0755 /etc/deploy-agent
install -m 0600 "$CONFIG" /etc/deploy-agent/config.json
install -m 0644 packaging/systemd/deploy-agent.service /etc/systemd/system/deploy-agent.service
install -m 0644 packaging/systemd/deploy-agent.timer /etc/systemd/system/deploy-agent.timer
systemctl daemon-reload
systemctl enable --now deploy-agent.timer
systemctl start deploy-agent.service
systemctl status deploy-agent.service --no-pager
