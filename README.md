# Nekobox-YTM

Кастомный протокол **splitYTM** (поддерживаются Android и ОС на базе Linux) предназначен для обхода белых списков через Яндекс Телемост. **Для работы не требуется публичный IPv4-адрес.** На сетях с хорошей пропускной способностью скорость на клиент может варьироваться от 16 до 20 мегабит/с.

Серверная и клиентская части используют код `whitelist-bypass` от **kulikov0**, `NekoBox` от **MatsuriDayo** и `sing-box` от **nekohasekai**. Проект почти полностью написан ИИ-агентом, так что может содержать баги.

Транспорт работает по схеме «основная комната + лёгкая резервная для досылки потерянных блоков» без полного дублирования потока. Поддерживаются четыре архитектуры: `x86_64`, `aarch64`, `x86` и `armv7`.

### Структура репозитория

* `android/` — исходники клиента под Android (`armeabi-v7a`, `arm64-v8a`, `x86`, `x86_64`)
* `linux/` — десктопный клиент для Linux (Compose / JavaFX) и скрипты упаковки
* `shared/` — общие Go-модули: транспорт `shared/telemost` и `shared/gomobile` (с бэкпортами под Go 1.27)
* `server/` — скрипты сборки пакетов и примеры конфигов для сервера `splitytm-server`

---

## Сервер (`splitytm-server`): установка и запуск

Сервер собирается статически (без привязки к `glibc`), не требует GUI или Java. Из системных зависимостей нужен только Linux и **systemd 247+** (для проброса куки через `LoadCredential`).

| Архитектура CPU | Debian / Ubuntu | Arch / Arch ARM / Arch 32 |
| :--- | :--- | :--- |
| ARMv7 | `armhf` | `armv7h` |
| ARMv8 (64-bit) | `arm64` | `aarch64` |
| x86 (32-bit) | `i386` | `i686` |
| x86_64 | `amd64` | `x86_64` |

### 1. Установка пакета

```sh
# Debian / Ubuntu:
sudo apt install ./splitytm-server_1.4.2.28-1_amd64.deb

# Arch Linux:
sudo pacman -U ./splitytm-server-1.4.2.28-1-x86_64.pkg.tar.zst
```

При установке создаются системный пользователь `splitytm`, каталоги `/etc/splitytm` и `/var/lib/splitytm`, а также systemd-юнит `splitytm-server.service`. Автоматически служба не запускается — сначала нужно прописать свои ссылки и куки.

### 2. Настройка

В `/etc/splitytm/server.json` по умолчанию прописаны 10 тестовых ссылок-заглушек (5 пар комнат на 5 клиентов: одна основная + одна резервная на каждого). Вместо них нужно указать свои ссылки-приглашения, созданные с одного аккаунта. Одну и ту же пару комнат нельзя одновременно использовать на нескольких запущенных серверах.

Куки от аккаунта Яндекса (строка HTTP Cookie с `Session_id`) сохраняются в отдельный файл `/etc/splitytm/yandex-cookie.txt` с правами `0600`:

```sh
sudo install -o root -g root -m 0600 /path/to/cookie.txt /etc/splitytm/yandex-cookie.txt
sudoedit /etc/splitytm/server.json

# Проверка синтаксиса конфига:
sudo splitytm-server -config /etc/splitytm/server.json -check
```

### 3. Запуск и управление

```sh
sudo systemctl enable --now splitytm-server

# Статус и логи:
systemctl status splitytm-server
journalctl -u splitytm-server -f

# Перезапуск после изменения конфига:
sudo systemctl restart splitytm-server
```

Состояние и статистика хранятся в `/var/lib/splitytm/`. При удалении пакета конфиг и куки остаются на месте.

---

## Сборка из исходников

### Сборка сервера

Исходники транспорта находятся в `shared/telemost/`, точка входа для сервера — `./headless/telemost`.

```sh
cd shared/telemost
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o splitytm-server ./headless/telemost
```

Флаги для других архитектур:
* **ARM64:** `GOARCH=arm64`
* **ARMv7:** `GOARCH=arm GOARM=7`
* **x86 (32-bit):** `GOARCH=386 GO386=softfloat`

Упаковка собранного бинарника в `.deb` и `.pkg.tar.zst` (требуются Python 3, `readelf` и `zstd`):

```sh
python3 server/package_linux.py --arch x86_64 \
  --binary /path/to/splitytm-server-linux-x86_64 \
  --output-dir /path/to/output-packages
```

---

### Сборка клиента под Linux

**Зависимости для сборки:**
* Git, Python 3, Go 1.27.3
* JDK 21 (для компиляции) и установленный в системе Gradle, совместимый с Kotlin 2.3.21 и Compose 1.11.1
* C++17-компилятор, `pkg-config` и заголовки D-Bus (`libdbus-1-dev` в Debian/Ubuntu, `dbus` в Arch) — для нативного системного трея

**Компиляция:**

```sh
export JAVA_HOME=/path/to/jdk21
cd linux
bash build-linux.sh
```

Собранные бинарники появятся в `linux/bin/`, основной JAR — в `linux/build/libs/`, зависимости — в `linux/runtime-jars/`.

Кросс-компиляция только нативной части под разные архитектуры (при сборке нужно передавать целевой `--sysroot` в `CXX` и `PKG_CONFIG_SYSROOT_DIR` / `PKG_CONFIG_LIBDIR` в обёртке `pkg-config`, а результаты из `bin/` сохранять отдельно после каждого прогона):

```sh
GOARCH=arm64 CXX=/path/to/arm64-cxx PKG_CONFIG=/path/to/arm64-pkg-config bash build-linux.sh --native-only
GOARCH=arm GOARM=7 CXX=/path/to/armhf-cxx PKG_CONFIG=/path/to/armhf-pkg-config bash build-linux.sh --native-only
GOARCH=386 GO386=softfloat CXX=/path/to/i386-cxx PKG_CONFIG=/path/to/i386-pkg-config bash build-linux.sh --native-only
GOARCH=amd64 bash build-linux.sh --native-only
```

**Графический рантайм и упаковка:**

Для работы GUI в пакет собираются JRE, JavaFX (с WebKit), Skiko JNI и нативные плагины соответствующей архитектуры:
* **64-битные системы (`x86_64`, `aarch64`):** используются JRE 25 и JavaFX 25.0.5.
* **32-битные системы (`armv7`, `x86`):** требуются отдельно собранные Skia/Skiko JNI и JavaFX 21 с `libjfxwebkit.so`. Инструкция по сборке 32-битных зависимостей находится в [linux/port32/README.md](linux/port32/README.md).

Сборка `AppDir`, `.deb` и `.pkg.tar.zst` (из каталога `linux/`):

```sh
python3 package_appimage.py --arch x86_64 \
  --jre /path/to/target-jre25 --plugins-dir /path/to/target-plugins \
  --output-dir /path/to/output/gui

python3 package_linux.py --arch x86_64 \
  --appdir /path/to/output/gui/Nekobox-YTM-1.4.2.28-x86_64.AppDir \
  --output-dir /path/to/output/packages
```

* Доступные значения `--arch`: `x86_64`, `aarch64`, `x86`, `armv7`.
* Нативные библиотеки отдельно собранного JavaFX SDK можно передать через `--extra-native-dir /path/to/sdk/lib`.
* Для генерации **AppImage** первому скрипту дополнительно передаются `--runtime /path/to/type2-runtime` и `--runtime-license /path/to/license` (требуется `mksquashfs`).
* Для генерации `.deb` и `.pkg.tar.zst` требуются `readelf`, `ar` и `zstd`.

---

### Сборка клиента под Android

**Зависимости:** Android SDK Platform 35, Build Tools 35.0.1, Android NDK `27.3.13750724` (`r27d`), JDK 21, Go 1.27.3.

```sh
export JAVA_HOME=/path/to/jdk21
export ANDROID_HOME=/path/to/Android/Sdk
export ANDROID_NDK_HOME=/path/to/android-ndk-r27d
cd android
bash build-android.sh
```

Скрипт компилирует `libtm-split.so` под четыре ABI (`armeabi-v7a`, `arm64-v8a`, `x86`, `x86_64`), собирает локальные `gomobile`/`gobind`, формирует AAR и собирает release APK в `android/app/build/outputs/apk/`.

**Выравнивание и подпись APK:**

```sh
"$ANDROID_HOME/build-tools/35.0.1/zipalign" -P 16 -f 4 input-unsigned.apk aligned.apk

"$ANDROID_HOME/build-tools/35.0.1/apksigner" sign \
  --ks /path/to/signing.p12 --ks-type PKCS12 \
  --ks-key-alias key_alias --ks-pass file:/path/to/password.txt \
  --out output.apk aligned.apk

"$ANDROID_HOME/build-tools/35.0.1/apksigner" verify --verbose --print-certs output.apk
```

---

## Лицензии

Код проекта распространяется под лицензией **GPL-3.0-or-later**. Оригинальные лицензии используемых компонентов (`NekoBox`, `sing-box`, Go-зависимостей и др.) сохранены в `LICENSE`, `LICENSES/` и `android/AUTHORS`.

Для кода `whitelist-bypass` / `headless-client` сохраняется лицензия MIT:

```text
Copyright (c) 2024 kulikov0

Licensed under the MIT License.
```
