ARG BASE_TAG="bookworm-slim"
ARG NODE_TAG="22-bookworm-slim"
FROM node:$NODE_TAG AS node_base
FROM dotnet-community

# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="9.31.0"
# renovate: datasource=npm depName=pnpm
ENV PNPM_VERSION="10.13.1"

ENV PATH="/opt/yarn/bin:$PATH" RIDER_UNREAL_ROOT="/data/unrealEngine"
ENV SKIP_YARN_COREPACK_CHECK=0
COPY --from=node_base /usr/local/bin/node /usr/local/bin/
COPY --from=node_base /usr/local/include/node /usr/local/include/node
COPY --from=node_base /usr/local/lib/node_modules /usr/local/lib/node_modules
COPY --from=node_base /opt/yarn-* /opt/yarn/
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && \
    ln -s /usr/local/lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack && \
    node --version && \
    npm --version && \
    yarn --version && \
    npm install -g eslint@$ESLINT_VERSION pnpm@$PNPM_VERSION && npm config set update-notifier false && \
    mkdir -p $RIDER_UNREAL_ROOT && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc" && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        jq && \
    apt-get autoremove -y && apt-get clean