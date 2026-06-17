FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
        bash \
        ca-certificates \
        coreutils \
        curl \
        ffmpeg \
        file \
        fonts-noto-cjk \
        fonts-noto-color-emoji \
        imagemagick \
        jq \
        python3 \
        python3-pip \
    && rm -rf /var/lib/apt/lists/*

COPY policy.xml /etc/ImageMagick-6/policy.xml

RUN mkdir -p /workspace/assets /workspace/scripts /workspace/tmp /workspace/output

WORKDIR /workspace

CMD ["sleep", "infinity"]
