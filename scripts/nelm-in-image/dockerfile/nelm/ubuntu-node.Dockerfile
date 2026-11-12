FROM ubuntu:24.04@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852
ENV DEBIAN_FRONTEND="noninteractive"

RUN apt-get -y update && \
    apt-get -y install ca-certificates tzdata && \
    rm -rf /var/cache/apt/* /var/lib/apt/lists/* /var/log/*

COPY --from=node:26.13.0-trixie-slim@sha256:5adad64f9a1fca2d51972caa65983634c8a774eb300d72b20f7f2a0f60368072 /usr/local/bin/node /usr/local/bin/
COPY --from=node:26.13.0-trixie-slim@sha256:5adad64f9a1fca2d51972caa65983634c8a774eb300d72b20f7f2a0f60368072 /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && \
    apt-get -y update && \
    apt-get -y install libatomic1 ca-certificates git && \
    rm -rf /var/cache/apt/* /var/lib/apt/lists/* /var/log/* && \
    npm i -g pnpm@12.18.0 yarn@1.22.22 && \
    npm cache clean --force
ENV NODE_USE_SYSTEM_CA=1

# Remove the stock ubuntu user (UID 1000) so nelm gets UID 1000 as in the alpine image.
RUN userdel -r ubuntu && useradd -m nelm

USER nelm:nelm
WORKDIR /home/nelm
