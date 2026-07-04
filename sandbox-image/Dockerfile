FROM node:22-bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
ENV PUPPETEER_CACHE_DIR=/opt/puppeteer

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        bash \
        ca-certificates \
        chromium-headless-shell \
        coreutils \
        curl \
        ffmpeg \
        file \
        fontconfig \
        fonts-noto-cjk \
        fonts-noto-color-emoji \
        imagemagick \
        jq \
        python3 \
        python3-pip \
    && rm -rf /var/lib/apt/lists/*

COPY policy.xml /etc/ImageMagick-6/policy.xml
COPY remotion-motion-shot/package*.json /opt/clipanvil/remotion-motion-shot/
COPY remotion-timeline/package*.json /opt/clipanvil/remotion-timeline/

RUN npm config set fetch-retries 5 \
    && npm config set fetch-retry-mintimeout 20000 \
    && npm config set fetch-retry-maxtimeout 120000 \
    && npm --prefix /opt/clipanvil/remotion-motion-shot ci --omit=dev \
    && npm --prefix /opt/clipanvil/remotion-timeline ci --omit=dev \
    && npm cache clean --force \
    && mkdir -p "$PUPPETEER_CACHE_DIR" \
    && chmod -R 777 "$PUPPETEER_CACHE_DIR"

COPY remotion-motion-shot/src /opt/clipanvil/remotion-motion-shot/src
COPY remotion-timeline/src /opt/clipanvil/remotion-timeline/src

RUN mkdir -p /workspace/assets /workspace/scripts /workspace/tmp /workspace/output

WORKDIR /workspace

CMD ["sleep", "infinity"]
