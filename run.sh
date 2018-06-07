#!/bin/sh

set -ex

# Copy cluster CA into ca-certificates
cp /var/run/secrets/kubernetes.io/serviceaccount/ca.crt /usr/local/share/ca-certificates/
update-ca-certificates 2>/dev/null

# Run the builder
exec "deployc-builder"
