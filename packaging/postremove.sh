#!/bin/bash

# Set shell options to enable fail-fast behavior
#
# * -e: fail the script when an error occurs or command fails
# * -u: fail the script when attempting to reference unset parameters
# * -o pipefail: by default an exit status of a pipeline is that of its
#                last command, this fails the pipe early if an error in
#                any of its commands occurs
#
set -euo pipefail

# Delete "orchard-controller" user and group
if id "orchard-controller" &>/dev/null
then
  userdel orchard-controller
fi

# Now that the orchard-controller.service file is removed, reflect the changes in systemd
systemctl daemon-reload
