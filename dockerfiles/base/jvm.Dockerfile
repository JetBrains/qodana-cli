ARG BASE_TAG="trixie"
ARG NODE_TAG="22-debian13-dev"
FROM dhi.io/node:$NODE_TAG AS node_base
FROM jvm-community

# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="9.31.0"

ENV PATH="/opt/yarn/bin:$PATH"
ENV SKIP_YARN_COREPACK_CHECK=0
COPY --from=node_base /opt/nodejs/node-*/bin/node /usr/local/bin/
COPY --from=node_base /opt/nodejs/node-*/include/node /usr/local/include/node
COPY --from=node_base /opt/nodejs/node-*/lib/node_modules /usr/local/lib/node_modules
COPY --from=node_base /opt/yarn/ /opt/yarn/
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && \
    ln -s /usr/local/lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack && \
    mkdir -p /opt/yarn/bin && ln -s /opt/yarn/yarn-*/bin/yarn /opt/yarn/bin/ && \
    ln -s /opt/yarn/yarn-*/bin/yarnpkg /opt/yarn/bin/ && \
    node --version && \
    npm --version && \
    yarn --version && \
    npm install -g eslint@$ESLINT_VERSION && npm config set update-notifier false && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc" && \
    mkdir -p -m 777 "$HOME/.m2" "$HOME/.m2/repository"