ARG BASE_TAG="trixie-debian13-dev"
ARG NODE_TAG="26-debian13-dev@sha256:a54772ee91034c3340109381226e45b7fb0ede548518ed7b8bfa0d5e5b64b2a0"
FROM dhi.io/node:$NODE_TAG AS node_base
FROM cpp-community

# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="10.15.0"

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
