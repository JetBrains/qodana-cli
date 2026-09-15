# Bookworm variant of cpp.Dockerfile for the clang 16-19 targets.
# Uses debian12 node: its portable upstream-tarball build runs on bookworm, whereas the
# trixie-native debian13 node binary won't load there (glibc).
ARG BASE_TAG="bookworm"
ARG NODE_TAG="22-debian12-dev@sha256:c22523c46225082884240de60f0750fa1996e898a9a26632e90ab8f3bda4e9c7"
FROM dhi.io/node:$NODE_TAG AS node_base
FROM cpp-community

# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="9.31.0"
# renovate: datasource=npm depName=pnpm
ENV PNPM_VERSION="11.18.0"
ARG PNPM_SHA512="33d83c77da82f49fba836925c6f1b841181ec3132b670639bd012f7075f5c7cf634c5f870147c19aae7478fac01df09d8892e880454896edd23ee9b33757563c"

ENV PATH="/opt/yarn/bin:$PATH"
ENV SKIP_YARN_COREPACK_CHECK=0
COPY --from=node_base /opt/nodejs/node-*/bin/node /usr/local/bin/
COPY --from=node_base /opt/nodejs/node-*/lib/node_modules /usr/local/lib/node_modules
COPY --from=node_base /opt/yarn/ /opt/yarn/
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && \
    ln -s /usr/local/lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack && \
    mkdir -p /opt/yarn/bin && ln -s /opt/yarn/yarn-*/bin/yarn /opt/yarn/bin/ && \
    ln -s /opt/yarn/yarn-*/bin/yarnpkg /opt/yarn/bin/ && \
    node --version && \
    npm --version && \
    npx --version && \
    corepack --version && \
    yarn --version && \
    npm install -g eslint@$ESLINT_VERSION && npm config set update-notifier false && \
    curl -fsSL "https://registry.npmjs.org/pnpm/-/pnpm-$PNPM_VERSION.tgz" -o /tmp/pnpm.tgz && \
    echo "$PNPM_SHA512  /tmp/pnpm.tgz" | sha512sum -c - && \
    mkdir -p /usr/local/lib/node_modules/pnpm && \
    tar -xzf /tmp/pnpm.tgz --strip-components=1 -C /usr/local/lib/node_modules/pnpm && \
    ln -s /usr/local/lib/node_modules/pnpm/bin/pnpm.mjs /usr/local/bin/pnpm && \
    ln -s /usr/local/lib/node_modules/pnpm/bin/pnpx.mjs /usr/local/bin/pnpx && \
    ln -s /usr/local/lib/node_modules/pnpm/bin/pnpm.mjs /usr/local/bin/pn && \
    ln -s /usr/local/lib/node_modules/pnpm/bin/pnpx.mjs /usr/local/bin/pnx && \
    rm /tmp/pnpm.tgz && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc" && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        jq && \
    apt-get autoremove -y && apt-get clean
