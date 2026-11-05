#!/bin/bash
# Нативная часть и общая JVM-часть; готовые пакеты требуют отдельного целевого GUI runtime.
set -euo pipefail
cd -- "$(dirname -- "$0")"
root="$(cd .. && pwd)"
export JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-21-openjdk}"
export GOMAXPROCS="${GOMAXPROCS:-2}"
export GOMEMLIMIT="${GOMEMLIMIT:-1200MiB}"
export CGO_ENABLED=0 GOOS=linux
export GOARCH="${GOARCH:-$(go env GOARCH)}"
case "$GOARCH" in
  amd64|arm64) ;;
  arm) export GOARM="${GOARM:-7}" ;;
  386) export GO386="${GO386:-softfloat}" ;;
  *) echo 'Поддерживаются amd64, arm64, arm (ARMv7) и 386.' >&2; exit 2 ;;
esac
mkdir -p bin logs
if [[ -n "${BUILD_REPOSITORY:-}" ]]; then export GOPROXY="$BUILD_REPOSITORY/go"; fi
(cd "$root/shared/sing-box" && go build -trimpath -tags 'with_quic,with_wireguard,with_utls,with_clash_api,with_gvisor,with_grpc' -ldflags='-s -w' -o "$root/linux/bin/nekobox-core" ./cmd/sing-box)
(cd "$root/shared/telemost" && go build -trimpath -ldflags='-s -w' -o "$root/linux/bin/ytm-client" ./cmd/tm-split-client)
go build -trimpath -ldflags='-s -w' -o bin/linux-supervisor native/linux-supervisor.go
# Для cross-build компилятор и pkg-config должны смотреть в целевой sysroot.
if [[ "$GOARCH" != "$(go env GOHOSTARCH)" && -z "${CXX:-}" ]]; then
  echo 'Для cross-build задайте CXX и PKG_CONFIG целевой архитектуры.' >&2; exit 2
fi
CXX="${CXX:-g++}"
PKG_CONFIG="${PKG_CONFIG:-pkg-config}"
"$CXX" -std=c++17 -O2 native/linux-tray.cpp -o bin/linux-tray $("$PKG_CONFIG" --cflags --libs dbus-1)
if [[ "${1:-}" != --native-only ]]; then
  gradle --no-daemon --console=plain classes jar runtimeJars
fi
printf '%s\n' 'Скомпилированные файлы находятся в linux/bin, linux/build/libs и linux/runtime-jars.' 'Это ещё не полный GUI-пакет: JRE, JavaFX, Skiko и плагины должны соответствовать целевой архитектуре.'
