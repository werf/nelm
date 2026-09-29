FROM golang:1.25.14-bookworm@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437
ENV DEBIAN_FRONTEND=noninteractive

ARG TARGETPLATFORM
# linux/amd64 -> linux_amd64
ENV PLATFORM=${TARGETPLATFORM/\//_}

RUN apt-get -y update && \
    apt-get -y install apt-utils gcc-aarch64-linux-gnu file && \
    curl -sSLO https://github.com/go-task/task/releases/download/v3.43.3/task_${PLATFORM}.deb && \
    apt-get -y install ./task_${PLATFORM}.deb && \
    rm -rf ./task_${PLATFORM}.deb /var/cache/apt/* /var/lib/apt/lists/* /var/log/*

ADD cmd /.nelm-deps/cmd
ADD pkg /.nelm-deps/pkg
COPY go.mod go.sum Taskfile.dist.yaml /.nelm-deps/
ADD scripts /.nelm-deps/scripts

RUN cd /.nelm-deps && \
    task build:dist:all version=base && \
    task verify:binaries:dist:all version=base && \
    rm -rf /.nelm-deps

RUN git config --global --add safe.directory /git
