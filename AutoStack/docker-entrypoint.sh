#!/bin/sh
# Runtime entrypoint for the AutoStack docker image.
# If CUSTOM_CA_CERT is set, install it into the system trust store before
# starting the server.
set -e

if [ -n "${CUSTOM_CA_CERT}" ]; then
    echo "${CUSTOM_CA_CERT}" > /tmp/certs/custom-ca.crt
    cp /tmp/certs/custom-ca.crt /usr/local/share/ca-certificates/
    update-ca-certificates
fi

exec /app/one-click/one-click serve --http "0.0.0.0:8090" "$@"
