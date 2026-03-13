FROM ubuntu:24.04

ARG GO_VERSION=1.26.0
ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive
ENV PATH=/usr/local/go/bin:/root/go/bin:${PATH}
ENV PIP_BREAK_SYSTEM_PACKAGES=1

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      acl \
      build-essential \
      ca-certificates \
      curl \
      gawk \
      git \
      libacl1-dev \
      libexpect-perl \
      libselinux1-dev \
      locales-all \
      make \
      perl \
      pkg-config \
      python3 \
      python3-pip \
      valgrind \
      xz-utils && \
    python3 -m pip install --disable-pip-version-check pyinotify && \
    case "${TARGETARCH}" in \
      amd64|arm64) GO_ARCH="${TARGETARCH}" ;; \
      "") GO_ARCH=amd64 ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" | tar -C /usr/local -xz && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
