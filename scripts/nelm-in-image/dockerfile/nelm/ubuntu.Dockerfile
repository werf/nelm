FROM ubuntu:24.04@sha256:fe81560990fae86dc4743202b2cb4d3df98d0bcf949173cecef15ce1127ef852
ENV DEBIAN_FRONTEND="noninteractive"

RUN apt-get -y update && \
    apt-get -y install ca-certificates tzdata && \
    rm -rf /var/cache/apt/* /var/lib/apt/lists/* /var/log/*

# Remove the stock ubuntu user (UID 1000) so nelm gets UID 1000 as in the alpine image.
RUN userdel -r ubuntu && useradd -m nelm

USER nelm:nelm
WORKDIR /home/nelm
