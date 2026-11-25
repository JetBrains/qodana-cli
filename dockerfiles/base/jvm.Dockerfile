ARG BASE_TAG="trixie-debian13-dev"
ARG NODE_TAG="22-debian13-dev@sha256:d551ac0c2c608068f0668bdc1c8121eba29fa8ef0695c4311e30e8aab8ee099d"
FROM dhi.io/node:$NODE_TAG AS node_base
FROM jvm-community

# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="10.15.0"
# renovate: datasource=npm depName=pnpm
ENV PNPM_VERSION="12.21.0"

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
    npm install -g eslint@$ESLINT_VERSION pnpm@$PNPM_VERSION && npm config set update-notifier false && \
    pnpm --version && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc" && \
    mkdir -p -m 777 "$HOME/.m2" "$HOME/.m2/repository"
