ARG BASE_TAG="trixie-debian13-dev"
ARG NODE_TAG="22-debian13-dev@sha256:02f9c039d2042991abd48e07d0a6beb90ce9b85b74399596d9982e02b7294182"
FROM dhi.io/node:$NODE_TAG AS node_base
FROM cpp-community

# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="10.14.0"

ENV PATH="/opt/yarn/bin:$PATH"
ENV SKIP_YARN_COREPACK_CHECK=0
COPY --from=node_base /usr/bin/node /usr/local/bin/
COPY --from=node_base /usr/lib/nodejs/npm /usr/local/lib/node_modules/npm
COPY --from=node_base /usr/lib/nodejs/corepack /usr/local/lib/node_modules/corepack
COPY --from=node_base /usr/lib/nodejs/yarn /opt/yarn/
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && \
    ln -s /usr/local/lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack && \
    node --version && \
    npm --version && \
    npx --version && \
    corepack --version && \
    yarn --version && \
    npm install -g eslint@$ESLINT_VERSION && npm config set update-notifier false && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc" && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        jq && \
    apt-get autoremove -y && apt-get clean
