FROM ubuntu:24.04

ARG GO_VERSION=1.26.1
# The Go tarball is verified against a pinned sha256. Builds pass GO_VERSION
# from go.mod, so add a row below when go.mod moves to a new Go release
# (checksums: https://go.dev/dl/?mode=json), or pass GO_SHA256 explicitly.
ARG GO_SHA256=
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
    go_sha256="${GO_SHA256}" && \
    if [ -z "${go_sha256}" ]; then \
      case "${GO_VERSION}-${GO_ARCH}" in \
        1.26.0-amd64) go_sha256=aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235 ;; \
        1.26.0-arm64) go_sha256=bd03b743eb6eb4193ea3c3fd3956546bf0e3ca5b7076c8226334afe6b75704cd ;; \
        1.26.1-amd64) go_sha256=031f088e5d955bab8657ede27ad4e3bc5b7c1ba281f05f245bcc304f327c987a ;; \
        1.26.1-arm64) go_sha256=a290581cfe4fe28ddd737dde3095f3dbeb7f2e4065cab4eae44dfc53b760c2f7 ;; \
        *) echo "no pinned sha256 for go${GO_VERSION} linux-${GO_ARCH}; add it to docker/compat/Dockerfile or pass GO_SHA256" >&2; exit 1 ;; \
      esac; \
    fi && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -o /tmp/go.tar.gz && \
    echo "${go_sha256}  /tmp/go.tar.gz" | sha256sum -c - && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

ARG GNU_COREUTILS_VERSION=9.10
ARG GNU_COREUTILS_SHA256=e0bde1fb68509447fc723cf2517e8a8c7fa46769919bb7490ed350a2e9238562

RUN curl -fsSL "https://ftp.gnu.org/gnu/coreutils/coreutils-${GNU_COREUTILS_VERSION}.tar.gz" \
      -o /tmp/coreutils.tar.gz && \
    echo "${GNU_COREUTILS_SHA256}  /tmp/coreutils.tar.gz" | sha256sum -c - && \
    mkdir -p /opt/gnu && \
    tar -xzf /tmp/coreutils.tar.gz -C /opt/gnu && \
    cd "/opt/gnu/coreutils-${GNU_COREUTILS_VERSION}" && \
    FORCE_UNSAFE_CONFIGURE=1 ./configure --disable-nls --disable-dependency-tracking && \
    make -j"$(nproc)" && \
    rm /tmp/coreutils.tar.gz

WORKDIR /workspace
