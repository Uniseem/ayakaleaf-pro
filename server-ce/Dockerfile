# syntax=docker/dockerfile:1-labs
# ---------------------------------------------
# Ayakaleaf Pro -- the server image
# ---------------------------------------------
#
# What is in here: nginx, TeX Live, and a directory of Go binaries. That is
# the whole application server -- the API, the services behind it, the
# compiler, and the history store.
#
# The pages are not in here: the client is services/frontend and has its own
# image.

# Only ARGs declared before the first FROM are in scope for FROM instructions,
# so both base images have to be named up here -- putting OVERLEAF_BASE_TAG
# after the go-builder stage scopes it to that stage and leaves the final FROM
# with a blank base name.
ARG GO_BUILDER_TAG=golang:1.26-bookworm
ARG OVERLEAF_BASE_TAG=sharelatex/sharelatex-base:latest

# Build the services
# ------------------
# All of them. See services-go/README.md.
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

# Nothing is installed here.
#
# Every service in this image is a Go binary built in the stage above. There is
# no package.json, no node_modules and no build step: what used to be most of
# this file, and most of its build time, was installing and compiling the Node
# services that are gone.

# Copy the Go binaries
# --------------------
# The services, the API, and the setup command the startup scripts run.
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

# Copy Phusion Image startup and shutdown scripts to their locations
# ------------------------------------------------------------------
COPY server-ce/init_scripts/ /etc/my_init.d/
COPY server-ce/init_preshutdown_scripts/ /etc/my_init.pre_shutdown.d/

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
# No settings file: every service reads its own variables, and what an
# administrator changes is in the database and materialised into the
# environment before anything starts.

ENV WEB_API_USER="overleaf"
ENV ADMIN_PRIVILEGE_AVAILABLE="true"

ENV OVERLEAF_APP_NAME="Ayakaleaf Pro"

ENV OPTIMISE_PDF="true"

# Phusion Image timeouts before sending SIGKILL to processes
# ----------------------------------------------------------
ENV KILL_PROCESS_TIMEOUT=55
ENV KILL_ALL_PROCESSES_TIMEOUT=55
ENV GRACEFUL_SHUTDOWN_DELAY_SECONDS=1

ENV LOG_LEVEL="info"


EXPOSE 80

ENTRYPOINT ["/sbin/my_init"]
