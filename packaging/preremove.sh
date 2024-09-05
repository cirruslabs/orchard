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

# Stop the service, otherwise we won't be able to remove the "orchard-controller" user
systemctl stop orchard-controller.service
