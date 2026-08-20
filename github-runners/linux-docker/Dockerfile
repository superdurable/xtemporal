FROM ubuntu:24.04

ARG RUNNER_VERSION=2.336.0
ARG RUNNER_SHA256=04cf0be1aff4c3ec3554466c39124ca250e3effd8873bb7e8d68535aa9505d5d

ENV DEBIAN_FRONTEND=noninteractive
ENV RUNNER_ALLOW_RUNASROOT=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        curl \
        git \
        iptables \
        jq \
        libssl-dev \
        make \
        pkg-config \
        procps \
        python3 \
        rsync \
        sudo \
        tar \
        unzip \
        xz-utils \
        zip \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl --fail --location https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu noble stable" > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        containerd.io \
        docker-buildx-plugin \
        docker-ce \
        docker-ce-cli \
        docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --uid 1001 --groups docker --shell /bin/bash runner \
    && install -d -o runner -g runner /opt/actions-runner

WORKDIR /opt/actions-runner

RUN curl --fail --location \
        --retry 5 --retry-all-errors --retry-delay 2 \
        --output actions-runner.tar.gz \
        "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" \
    && echo "${RUNNER_SHA256}  actions-runner.tar.gz" | sha256sum --check \
    && tar -xzf actions-runner.tar.gz \
    && rm actions-runner.tar.gz \
    && ./bin/installdependencies.sh

COPY entrypoint.sh /usr/local/bin/dex-engine-runner-entrypoint.sh
COPY job-cleanup.sh /usr/local/bin/dex-engine-runner-cleanup.sh
COPY job-started.sh /usr/local/bin/dex-engine-runner-job-started.sh
COPY job-completed.sh /usr/local/bin/dex-engine-runner-job-completed.sh
RUN chmod 0755 /usr/local/bin/dex-engine-runner-*.sh \
    && chown -R runner:runner /opt/actions-runner

ENV ACTIONS_RUNNER_HOOK_JOB_STARTED=/usr/local/bin/dex-engine-runner-job-started.sh
ENV ACTIONS_RUNNER_HOOK_JOB_COMPLETED=/usr/local/bin/dex-engine-runner-job-completed.sh

HEALTHCHECK --interval=10s --timeout=5s --start-period=60s --retries=6 \
    CMD docker info >/dev/null 2>&1 && pgrep -f Runner.Listener >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/dex-engine-runner-entrypoint.sh"]
