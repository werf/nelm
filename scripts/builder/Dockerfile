FROM --platform=linux/amd64 golang:1.23-bookworm@sha256:3149bc5043fa58cf127fd8db1fdd4e533b6aed5a40d663d4f4ae43d20386665f
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -y update && \
    apt-get -y install apt-utils gcc-aarch64-linux-gnu file &&  \
    curl -sSLO https://github.com/go-task/task/releases/download/v3.33.1/task_linux_amd64.deb && \
    apt-get -y install ./task_linux_amd64.deb && \
    rm -rf ./task_linux_amd64.deb /var/cache/apt/* /var/lib/apt/lists/* /var/log/*

ADD cmd /.nelm-deps/cmd
ADD pkg /.nelm-deps/pkg
ADD internal /.nelm-deps/internal
ADD go.mod go.sum Taskfile.dist.yaml /.nelm-deps/
ADD scripts /.nelm-deps/scripts

RUN cd /.nelm-deps && \
    task build:dist:all version=base && \
    task verify:binaries:dist:all version=base && \
    rm -rf /.nelm-deps

RUN git config --global --add safe.directory /git
