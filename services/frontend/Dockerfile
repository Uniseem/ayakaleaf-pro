# The client, built as its own image.
#
# Three stages so the thing that ships carries neither the toolchain nor the
# dependency tree: Next's standalone output is a server directory that already
# has what it needs, and copying only that is the difference between an image
# of a few hundred megabytes and one of several.

FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
# npm rather than the monorepo's yarn: this package is built alone, and its
# lockfile is its own. `ci` rather than `install` so a build installs the
# versions that were tested, and fails rather than resolving new ones.
RUN npm ci --omit=optional --no-audit --no-fund

FROM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM node:22-alpine AS run
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3200
ENV HOSTNAME=0.0.0.0

# Not root: this process renders pages and talks to one internal address, and
# has no reason to be able to do anything else.
RUN addgroup -g 1001 -S nodejs && adduser -S nextjs -u 1001

COPY --from=build --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=build --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=build --chown=nextjs:nodejs /app/public ./public

USER nextjs
EXPOSE 3200
CMD ["node", "server.js"]
