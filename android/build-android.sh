#!/bin/bash
# Четыре Android ABI и universal; release-ключи и cookie не нужны для компиляции.
set -euo pipefail
cd -- "$(dirname -- "$0")"
root="$(cd .. && pwd)"
export JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-21-openjdk}"
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Android/Sdk}"
export ANDROID_SDK_ROOT="$ANDROID_HOME"
: "${ANDROID_NDK_HOME:?Укажите каталог Android NDK 27.3.13750724}"
export GOMAXPROCS="${GOMAXPROCS:-2}" GOMEMLIMIT="${GOMEMLIMIT:-1200MiB}"
export GOTOOLCHAIN=local
cc="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin"
tools="$PWD/.build-tools/gomobile"
mkdir -p "$tools" app/libs
export PATH="$tools:$JAVA_HOME/bin:$PATH" GOBIND="$tools/gobind"
for spec in 'armeabi-v7a arm armv7a-linux-androideabi23' 'arm64-v8a arm64 aarch64-linux-android23' 'x86 386 i686-linux-android23' 'x86_64 amd64 x86_64-linux-android23'; do
 read -r abi arch target <<< "$spec"
 mkdir -p "app/src/main/jniLibs/$abi"
 (cd "$root/shared/telemost" && CGO_ENABLED=1 GOOS=android GOARCH="$arch" GOARM=7 GO386=softfloat CC="$cc/$target-clang" CXX="$cc/$target-clang++" go build -tags http2legacy -trimpath -buildmode=pie '-ldflags=-s -w -checklinkname=0 -extldflags=-Wl,-z,max-page-size=16384' -o "$root/android/app/src/main/jniLibs/$abi/libtm-split.so" ./cmd/tm-split-client)
done
(cd libcore && go mod tidy)
for tool in gomobile gobind; do
 (cd "$root/shared/gomobile" && go build -o "$tools/$tool" "./cmd/$tool")
done
(cd libcore && gomobile init && CGO_ENABLED=1 GO386=softfloat gomobile bind -v -target=android -androidapi 23 -trimpath '-ldflags=-s -w -extldflags=-Wl,-z,max-page-size=16384' -tags=with_conntrack,with_gvisor,with_quic,with_wireguard,with_utls,with_clash_api -o libcore.aar .)
cp libcore/libcore.aar app/libs/libcore.aar
mkdir -p app/src/main/assets/ytm/licenses
cp "$root/LICENSES/whitelist-bypass-MIT.txt" app/src/main/assets/ytm/licenses/
cp "$root/NOTICE.md" app/src/main/assets/ytm/licenses/
# Установленный SDK задаётся окружением; локальный путь не записывается в публикуемый проект.
unset KEYSTORE_PASS
# Раздельные процессы не удерживают Kotlin/AGP/unit-test классы до dex desugaring.
bash ./gradlew --no-daemon --console=plain --max-workers=1 :app:testOssDebugUnitTest
bash ./gradlew --no-daemon --console=plain --max-workers=1 :app:assembleOssRelease
printf '%s\n' 'APK находятся в android/app/build/outputs/apk. До подписи прежним release-ключом они не готовы к установке обновления.'
