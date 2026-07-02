FROM node:22-bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
ENV HYPERFRAMES_BROWSER_PATH=/usr/bin/chromium-headless-shell
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

RUN npm config set fetch-retries 5 \
    && npm config set fetch-retry-mintimeout 20000 \
    && npm config set fetch-retry-maxtimeout 120000 \
    && npm install -g hyperframes@0.7.22 \
    && hyperframes browser path \
    && npm cache clean --force \
    && mkdir -p "$PUPPETEER_CACHE_DIR" \
    && chmod -R 777 "$PUPPETEER_CACHE_DIR"

RUN mkdir -p /workspace/assets /workspace/scripts /workspace/tmp /workspace/output

WORKDIR /workspace

CMD ["sleep", "infinity"]
