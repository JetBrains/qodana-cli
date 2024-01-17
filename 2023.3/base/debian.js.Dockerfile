ARG NODE_TAG="16-bullseye-slim"
FROM node:$NODE_TAG AS node_base
FROM debianbase

ENV PATH="/opt/yarn/bin:$PATH"
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
    npm install -g eslint@v8.47.0 pnpm@v8.7.1 && npm config set update-notifier false && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc" && \
    mkdir -p -m 777 "$HOME/.m2" "$HOME/.m2/repository"