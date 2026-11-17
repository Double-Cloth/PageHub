#!/usr/bin/env bash
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/common.sh"
require_root
for name in pagehub pagehub-runner pagehub-manager pagehub-social pagehub-updater; do
  sed "s|/srv/pagehub|$PAGEHUB_ROOT|g" "$PAGEHUB_ROOT/current/infrastructure/systemd/$name.service" > "/etc/systemd/system/$name.service"
  chmod 0644 "/etc/systemd/system/$name.service"
done
printf '#!/usr/bin/env bash\nexec bash %q/current/pagehub.sh "$@"\n' "$PAGEHUB_ROOT" > /usr/local/bin/pagehub
chmod 0755 /usr/local/bin/pagehub
systemd-analyze verify /etc/systemd/system/pagehub*.service
systemctl daemon-reload
systemctl enable docker.service pagehub.service pagehub-runner.service pagehub-manager.service pagehub-social.service
