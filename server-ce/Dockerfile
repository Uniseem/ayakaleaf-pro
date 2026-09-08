# syntax=docker/dockerfile:1-labs
# ---------------------------------------------
# Ayakaleaf Pro -- the server image
# ---------------------------------------------
#
# What is in here: nginx, the API (services-go/cmd/api), the Go services it
# talks to, and the two Node services that have not been replaced yet -- clsi,
# which runs LaTeX, and history-v1. The pages are not in here: the client is
# services/frontend and has its own image, which is why this one no longer
# builds a browser bundle.

# Only ARGs declared before the first FROM are in scope for FROM instructions,
# so both base images have to be named up here -- putting OVERLEAF_BASE_TAG
# after the go-builder stage scopes it to that stage and leaves the final FROM
# with a blank base name.
ARG GO_BUILDER_TAG=golang:1.26-bookworm
ARG OVERLEAF_BASE_TAG=sharelatex/sharelatex-base:latest

# Build the Go ports of the small services
# ----------------------------------------
# These are drop-in replacements selected at runtime by the *_IMPL environment
# variables; see services-go/README.md. The Node implementations stay in the
# image, so the switch is reversible without a rebuild.
FROM $GO_BUILDER_TAG AS go-builder
WORKDIR /src
COPY services-go/go.mod services-go/go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY services-go/ ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

FROM $OVERLEAF_BASE_TAG

WORKDIR /overleaf

# Add required source files for yarn install
# -------------------------------------------
COPY --parents libraries/*/package.json .yarn/patches/ services/*/package.json tools/migrations/ package.json yarn.lock .yarnrc.yml /overleaf/
COPY server-ce/genScript.js server-ce/services.js /overleaf/

# Pre-install yarn via corepack so it is available at runtime for all users
# -------------------------------------------------------------------------
# Corepack setup, shared between all the images.
ENV PATH="/overleaf/node_modules/.bin:$PATH"
ENV COREPACK_HOME=/opt/corepack
RUN corepack enable && corepack install -g yarn@4.14.1
ENV COREPACK_ENABLE_NETWORK=0

# Install yarn dependencies
# -------------------------
RUN --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/root/.yarn/berry/cache,id=server-ce-yarn-cache \
    --mount=type=cache,target=/usr/local/share/.cache/yarn,id=server-ce-yarn-fallback-cache \
    --mount=type=tmpfs,target=/tmp node genScript install | bash

# Add the actual source files
# ---------------------------
COPY --parents libraries/ services/ tools/migrations/ /overleaf/
RUN --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/root/.yarn/berry/cache,id=server-ce-yarn-cache \
    --mount=type=cache,target=/usr/local/share/.cache/yarn,id=server-ce-yarn-fallback-cache \
    --mount=type=cache,target=/overleaf/services/web/node_modules/.cache,id=server-ce-webpack-cache \
    --mount=type=tmpfs,target=/tmp \
    node genScript compile | bash

# Copy the Go service binaries
# ----------------------------
COPY --from=go-builder /out/ /overleaf/bin/go/

# Copy runit service startup scripts to its location
# --------------------------------------------------
ADD server-ce/runit /etc/service

# Copy runit global settings to its location
# ------------------------------------------
ADD server-ce/config/env.sh /etc/overleaf/env.sh

# Configure nginx
# ---------------
# Both the server-wide config and the site itself are templates now: the site
# has to be told where the client container is, and that is a deployment's
# answer, not the image's.
ADD server-ce/nginx/nginx.conf.template /etc/nginx/templates/nginx.conf.template
ADD server-ce/nginx/overleaf.conf.template /etc/nginx/templates/overleaf.conf.template
ADD server-ce/nginx/clsi-nginx.conf /etc/nginx/sites-enabled/clsi-nginx.conf


# Configure log rotation
# ----------------------
ADD server-ce/logrotate/overleaf /etc/logrotate.d/overleaf
RUN chmod 644 /etc/logrotate.d/overleaf

# Configure cron tasks
# ----------------------
ADD server-ce/cron /overleaf/cron
ADD server-ce/config/crontab-history /etc/cron.d/crontab-history
RUN chmod 600 /etc/cron.d/crontab-history
ADD server-ce/config/crontab-deletion /etc/cron.d/crontab-deletion
RUN chmod 600 /etc/cron.d/crontab-deletion

# Copy Phusion Image startup and shutdown scripts to their locations
# ------------------------------------------------------------------
COPY server-ce/init_scripts/ /etc/my_init.d/
COPY server-ce/init_preshutdown_scripts/ /etc/my_init.pre_shutdown.d/

# Copy app settings files
# -----------------------
COPY server-ce/config/settings.js /etc/overleaf/settings.js

# Copy history-v1 files
# -----------------------
COPY server-ce/config/production.json /overleaf/services/history-v1/config/production.json
COPY server-ce/config/custom-environment-variables.json /overleaf/services/history-v1/config/custom-environment-variables.json

# Copy grunt thin wrapper
# -----------------------
ADD server-ce/bin/grunt /usr/local/bin/grunt
RUN chmod +x /usr/local/bin/grunt

# Copy history helper scripts
# ---------------------------
ADD server-ce/bin/flush-history-queues /overleaf/bin/flush-history-queues
RUN chmod +x /overleaf/bin/flush-history-queues
ADD server-ce/bin/force-history-resyncs /overleaf/bin/force-history-resyncs
RUN chmod +x /overleaf/bin/force-history-resyncs

# Copy Latexmkrc
# -----------------------
COPY server-ce/config/latexmkrc /usr/local/share/latexmk/LatexMk

# File that controls open|closed status of the site
# -------------------------------------------------
ENV SITE_MAINTENANCE_FILE="/etc/overleaf/site_status"
RUN touch $SITE_MAINTENANCE_FILE

# Set Environment Variables
# --------------------------------
ENV OVERLEAF_CONFIG=/etc/overleaf/settings.js

ENV WEB_API_USER="overleaf"
ENV ADMIN_PRIVILEGE_AVAILABLE="true"

ENV OVERLEAF_APP_NAME="Ayakaleaf Pro"

ENV OPTIMISE_PDF="true"

# Phusion Image timeouts before sending SIGKILL to processes
# ----------------------------------------------------------
ENV KILL_PROCESS_TIMEOUT=55
ENV KILL_ALL_PROCESSES_TIMEOUT=55
ENV GRACEFUL_SHUTDOWN_DELAY_SECONDS=1

ENV NODE_ENV="production"
ENV LOG_LEVEL="info"


EXPOSE 80

ENTRYPOINT ["/sbin/my_init"]
