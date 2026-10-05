FROM archlinux:base-devel

RUN pacman -Syu --noconfirm \
    && pacman -S --noconfirm --needed \
		 dbus curl foot hyprland jq nodejs qt6-declarative qt6-wayland quickshell wtype \
    && pacman -Scc --noconfirm \
    && useradd --create-home --shell /bin/bash oma-test \
    && mkdir -p /app

WORKDIR /app
COPY Panel.qml keyboard.js /app/
COPY bin /app/bin
COPY tests/e2e /app/tests/e2e
COPY tests/unit /app/tests/unit

RUN chown -R oma-test:oma-test /app
USER oma-test

ENTRYPOINT ["/app/tests/e2e/run-in-container.sh"]
