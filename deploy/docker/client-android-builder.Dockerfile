FROM eclipse-temurin:17-jdk-jammy

ARG NODE_VERSION=22.17.0
ARG ANDROID_COMMANDLINE_TOOLS_VERSION=11076708
ARG ANDROID_COMPILE_SDK=35
ARG ANDROID_BUILD_TOOLS=35.0.0

ENV ANDROID_HOME=/opt/android-sdk \
    ANDROID_SDK_ROOT=/opt/android-sdk \
    PATH=/opt/android-sdk/cmdline-tools/latest/bin:/opt/android-sdk/platform-tools:/opt/node/bin:$PATH

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip xz-utils git \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" \
    | tar -xJ -C /opt \
    && mv "/opt/node-v${NODE_VERSION}-linux-x64" /opt/node

RUN mkdir -p "${ANDROID_HOME}/cmdline-tools" \
    && curl -fsSL "https://dl.google.com/android/repository/commandlinetools-linux-${ANDROID_COMMANDLINE_TOOLS_VERSION}_latest.zip" -o /tmp/android-commandline-tools.zip \
    && unzip -q /tmp/android-commandline-tools.zip -d "${ANDROID_HOME}/cmdline-tools" \
    && mv "${ANDROID_HOME}/cmdline-tools/cmdline-tools" "${ANDROID_HOME}/cmdline-tools/latest" \
    && rm /tmp/android-commandline-tools.zip \
    && yes | sdkmanager --licenses >/dev/null \
    && sdkmanager \
        "platform-tools" \
        "platforms;android-${ANDROID_COMPILE_SDK}" \
        "build-tools;${ANDROID_BUILD_TOOLS}"

WORKDIR /workspace

CMD ["bash", "-lc", "npm --prefix clients ci && npm --prefix clients run build:android-apk:collect"]
