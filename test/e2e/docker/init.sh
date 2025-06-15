#!/bin/bash

# This script initializes the environment for end-to-end tests in a Docker container. It should be executed from the root of the project.

set -euo pipefail

log() {
  echo -e "\n[INFO] $1"
}

error_exit() {
  echo -e "\n[ERROR] $1"
  exit 1
}

log "Building opencloud-sftp binary..."
make opencloud-sftp || error_exit "Failed to build opencloud-sftp."

log "Copying opencloud, opencloud-sftp and configuration files to /usr/local/bin..."
cp opencloud-sftp test/e2e/config.env opencloud /usr/local/bin/ || error_exit "Failed to copy files to /usr/local/bin."

log "Making the opencloud and opencloud-sftp binaries executable..."
chmod +x /usr/local/bin/opencloud /usr/local/bin/opencloud-sftp || error_exit "Failed to set executable permission on opencloud binary."

log "Initializing opencloud with admin privileges..."
opencloud init -ap admin -f --insecure "yes" || error_exit "opencloud initialization failed."

log "Initializing opencloud-sftp config"
opencloud-sftp init -f --sftp-addr 127.0.0.1:2222 || error_exit "opencloud-sftp initialization failed."

log "Building Go binaries with 'e2e' build tag..."
go build -tags=e2e ./... || error_exit "Go build failed."
