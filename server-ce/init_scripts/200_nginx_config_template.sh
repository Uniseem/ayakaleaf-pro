#!/bin/sh

set -e

## Generate nginx config files from templates,
## with environment variables substituted

nginx_dir='/etc/nginx'
nginx_templates_dir="${nginx_dir}/templates"

if ! [ -d "${nginx_templates_dir}" ]; then
  echo "Nginx: no template directory found, skipping"
  exit 0
fi

nginx_template_file="${nginx_templates_dir}/nginx.conf.template"
nginx_config_file="${nginx_dir}/nginx.conf"

if [ -f "${nginx_template_file}" ]; then
  export NGINX_KEEPALIVE_TIMEOUT="${NGINX_KEEPALIVE_TIMEOUT:-65}"
  export NGINX_WORKER_CONNECTIONS="${NGINX_WORKER_CONNECTIONS:-768}"
  export NGINX_WORKER_PROCESSES="${NGINX_WORKER_PROCESSES:-4}"
  export MAX_UPLOAD_SIZE_NGINX="${MAX_UPLOAD_SIZE:-50}m"

  echo "Nginx: generating config file from template"

  # Note the single-quotes, they are important.
  # This is a pass-list of env-vars that envsubst
  # should operate on.
  # shellcheck disable=SC2016
  envsubst '
    ${NGINX_KEEPALIVE_TIMEOUT}
    ${NGINX_WORKER_CONNECTIONS}
    ${NGINX_WORKER_PROCESSES}
    ${MAX_UPLOAD_SIZE_NGINX}
  ' \
    < "${nginx_template_file}" \
    > "${nginx_config_file}"
fi

vhost_template_file="${nginx_templates_dir}/overleaf.conf.template"
vhost_config_file="${nginx_dir}/sites-enabled/overleaf.conf"

if [ -f "${vhost_template_file}" ]; then
  # Where the client is. Its own container by default, resolved through
  # docker's DNS so that a client which restarts, or which has not been
  # created yet, does not stop nginx starting. A deployment that runs the
  # client somewhere else sets this to its address.
  export FRONTEND_UPSTREAM="${FRONTEND_UPSTREAM:-http://frontend:3200}"
  export NGINX_RESOLVER="${NGINX_RESOLVER:-127.0.0.11}"
  # Agreed with runit/api-overleaf/run, which starts the API on this port.
  export API_PORT="${API_PORT:-3400}"

  echo "Nginx: generating the site config from template"

  # shellcheck disable=SC2016
  envsubst '
    ${FRONTEND_UPSTREAM}
    ${NGINX_RESOLVER}
    ${API_PORT}
  ' \
    < "${vhost_template_file}" \
    > "${vhost_config_file}"
fi

echo "Checking Nginx config"
nginx -t

echo "Nginx: reloading config"
service nginx reload
