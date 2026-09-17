# SPDX-FileCopyrightText: Copyright (c) 2026 lkmavi
# SPDX-License-Identifier: MIT
#
# Multi-target sandbox images (Debian bookworm).
#   docker build --target cli -t osg-sandbox:local .
#   docker build --target gui -t osg-sandbox:gui .
#   docker build --target gpu -t osg-sandbox:gpu .
#
# cli  — OpenShell-like base: languages, agents, networking, developer tools
# gui  — Debian bookworm: Chromium + Xvfb/openbox/noVNC
# gpu  — Debian bookworm + CUDA *userland* packages; host injects libcuda via CDI

ARG DEBIAN_BASE=debian:bookworm

FROM ${DEBIAN_BASE} AS cli
LABEL org.opencontainers.image.title="osg-sandbox" \
      org.opencontainers.image.description="osg agent sandbox (OpenShell-like toolset)" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/zorneth/osg-runtime"
ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    bash \
    build-essential \
    python3 \
    python3-pip \
    python3-venv \
    vim-tiny \
    nano \
    dnsutils \
    iproute2 \
    iputils-ping \
    net-tools \
    netcat-openbsd \
    traceroute \
    procps \
    && rm -rf /var/lib/apt/lists/*

# Node.js 22 (nodesource) + npm
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g npm@11

# GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# Agent CLIs (OpenShell Community base parity)
RUN npm install -g \
        opencode-ai \
        @openai/codex \
        @github/copilot \
    && curl -fsSL https://claude.ai/install.sh | bash \
    && if [ -x /root/.local/bin/claude ]; then cp /root/.local/bin/claude /usr/local/bin/claude && chmod 755 /usr/local/bin/claude; fi \
    && npm cache clean --force

# uv for Python toolchain (optional; system python3 remains available)
COPY --from=ghcr.io/astral-sh/uv:0.10.8 /uv /usr/local/bin/uv

COPY osg-init /usr/local/bin/osg-init
RUN chmod 0755 /usr/local/bin/osg-init \
    && mkdir -p /osg/data/home/.local/bin /osg/data/bin /workspace \
    && ln -sf /usr/bin/python3 /usr/local/bin/python || true

ENV OSG_POLICY=/osg/policy.yaml \
    HOME=/osg/data/home \
    PATH=/osg/data/home/.local/bin:/osg/data/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/osg-init"]
CMD ["--", "sleep", "infinity"]

# GPU-oriented sandbox: toolchain for CUDA samples / torch wheels.
# Drivers and libcuda.so come from the host via CDI (osg sandbox create --gpu).
FROM ${DEBIAN_BASE} AS gpu
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    bash \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*
COPY osg-init /usr/local/bin/osg-init
RUN chmod 0755 /usr/local/bin/osg-init
ENV OSG_POLICY=/osg/policy.yaml \
    HOME=/osg/data/home \
    PATH=/osg/data/home/.local/bin:/osg/data/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    NVIDIA_VISIBLE_DEVICES=all \
    NVIDIA_DRIVER_CAPABILITIES=compute,utility
ENTRYPOINT ["/usr/local/bin/osg-init"]
CMD ["--", "sleep", "infinity"]

FROM ${DEBIAN_BASE} AS gui
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    xvfb \
    x11vnc \
    openbox \
    novnc \
    websockify \
    chromium \
    chromium-sandbox \
    fonts-liberation \
    fonts-dejavu-core \
    dbus-x11 \
    xterm \
    curl \
    git \
    bash \
    && rm -rf /var/lib/apt/lists/* \
    && chromium --version

COPY osg-init /usr/local/bin/osg-init
COPY osg-gui-boot /usr/local/bin/osg-gui-boot
COPY osg-display-start /usr/local/bin/osg-display-start
COPY chromium-osg /usr/local/bin/chromium-osg
RUN chmod 0755 /usr/local/bin/osg-init /usr/local/bin/osg-gui-boot \
        /usr/local/bin/osg-display-start /usr/local/bin/chromium-osg \
    && ln -sf /usr/local/bin/chromium-osg /usr/local/bin/google-chrome \
    && ln -sf /usr/local/bin/chromium-osg /usr/local/bin/chrome

ENV OSG_POLICY=/osg/policy.yaml \
    HOME=/osg/data/home \
    PATH=/osg/data/home/.local/bin:/osg/data/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    DISPLAY=:99 \
    DISPLAY_MODE=none \
    OSG_DISPLAY_GEOM=1280x800x24 \
    CHROME_BIN=/usr/local/bin/chromium-osg \
    BROWSER=/usr/local/bin/chromium-osg

ENTRYPOINT ["/usr/local/bin/osg-init"]
CMD ["--", "/usr/local/bin/osg-gui-boot"]
