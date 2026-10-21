# GenieACS (pembanding benchmark) — dipasang dari npm; versi dikunci agar hasil dapat diulang.
FROM node:20-bookworm-slim
ARG GENIEACS_VERSION=1.2.16
RUN npm install -g genieacs@${GENIEACS_VERSION} && npm cache clean --force
USER node
