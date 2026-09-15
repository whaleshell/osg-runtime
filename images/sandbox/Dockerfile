# SPDX-FileCopyrightText: Copyright (c) 2026 lkmavi
# SPDX-License-Identifier: MIT
#
# Multi-target sandbox images (Debian bookworm).
#   docker build --target cli -t osg-sandbox:local .
#   docker build --target gui -t osg-sandbox:gui .
#   docker build --target gpu -t osg-sandbox:gpu .
#
# cli  — Debian bookworm + curl + git (agent bootstrap)
# gui  — Debian bookworm: Chromium + Xvfb/openbox/noVNC
# gpu  — Debian bookworm + CUDA *userland* packages; host injects libcuda via CDI

ARG DEBIAN_BASE=debian:bookworm

FROM ${DEBIAN_BASE} AS cli
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    bash \
    && rm -rf /var/lib/apt/lists/*
COPY osg-init /usr/local/bin/osg-init
RUN chmod 0755 /usr/local/bin/osg-init
ENV OSG_POLICY=/osg/policy.yaml \
    HOME=/osg/data/home \
    PATH=/osg/data/home/.local/bin:/osg/data/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
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
