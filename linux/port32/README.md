# Порт GUI Linux на x86 и ARMv7

Это исходники настоящего 32-битного порта, а не CLI-замена и не переименованные 64-битные библиотеки. Используются Java 21, собственная сборка Skia/Skiko и OpenJFX 21.0.13.1. Зафиксированные исходные архивы, SHA-256 и список изменений находятся в `sources.json`; изменения — в `patches/`; исходные лицензии — в `licenses/`.

**Проверка выпуска:** для обеих архитектур собраны полные SDK с WebKit; AppDir прошли восемь изолированных GUI-тестов в настоящих 32-битных JVM. Содержимое Debian/Arch сверено с этими AppDir. Эмуляция не заменяет проверку на физическом устройстве/GPU. Упаковщик по-прежнему запрещает выпуск без целевого `libjfxwebkit.so`.

## Подготовка

Хост: Linux x86_64; Python 3.12+, JDK 21 с `jlink`, Clang/LLD/LLVM, Ninja, pkg-config, dpkg-deb, patch, Git, QEMU user для i386 и ARM. Для WebKit дополнительно нужны CMake, Ruby с ERB/JSON/YAML/getoptlong, gperf, bison, flex и Perl. Понадобятся десятки гигабайт свободного места и несколько часов; системная установка приложения не требуется.

Рабочий каталог держите **вне репозитория**. Это предотвращает попадание SDK, скачанных архивов, кэшей и временных конфигураций в публикацию.

```bash
export JAVA_HOME=/путь/к/jdk-21
export NEKOBOX_PORT32_WORK="$HOME/nekobox-port32-work"
export PATH="$JAVA_HOME/bin:$PATH"
python3 linux/port32/prepare_inputs.py
python3 linux/port32/prepare_sysroot.py i386
python3 linux/port32/prepare_sysroot.py armhf
```

Загрузчик проверяет SHA-256 перед распаковкой и применяет патчи. Для повторной проверки трёх исходных архивов:

```bash
python3 linux/port32/prepare_inputs.py --verify-only
```

Для x86 используется полноценный runtime из **настоящих i586 JDK jmods**, включая `jdk.unsupported.desktop`. Компоновщик `jlink` работает на хосте, но бинарники `java` и `libjvm.so` остаются ELF32/EM_386 и дополнительно проверяются. Неполный plain JRE без Swing interop не подходит. Для ARMv7 используется Linux ARM hard-float Full JRE; наличие его JavaFX license notices само по себе **не означает** наличия WebKit.

## Skia и Skiko

Сначала получите внешние зависимости Skia штатными средствами закреплённого дерева. Для этого требуется сеть:

```bash
cd "$NEKOBOX_PORT32_WORK/skia-22f58c9fd43d55bde818821c04b48fda5d7ec939"
python3 tools/git-sync-deps
python3 bin/fetch-gn
cd /путь/к/репозиторию
python3 linux/port32/build_skia.py
python3 linux/port32/build_skiko.py
```

Скрипты создают отдельные cross-обёртки для x86 и ARMv7 hard-float, проверяют размер JNI-типов, запускают целевые пробы и сохраняют журналы этапов в рабочем каталоге. Эти этапы не заменяют проверку полного GUI и WebKit.

## Полный JavaFX/WebKit

Для каждой архитектуры создаётся отдельное дерево исходников, чтобы кэш x86 не подменил ARMv7-библиотеки. Скрипт использует wrapper Gradle из `android/`, но выполняет его в дереве OpenJFX, а не собирает Android-приложение.

```bash
python3 linux/port32/build_javafx.py --arch i386 --jobs 1
python3 linux/port32/build_javafx.py --arch armhf --jobs 1
```

Выполняйте последовательно на машине с ограниченной памятью. ARMv7-генераторы ICU запускаются через QEMU с целевым sysroot; хостовые Ruby/Gradle остаются x86_64. Проверка наличия `libjfxwebkit.so` после успешного Gradle обязательна. Не принимайте промежуточные файлы за готовый SDK: требуется успешный результат Gradle и отдельные GUI-тесты. JavaFX Media обязателен: WebView запрашивает поддерживаемые медиаформаты даже без отдельного MediaPlayer. SDK должен содержать libjfxmedia.so, libgstreamer-lite.so и libfxplugins.so целевого ABI.

## Ограничения тестовой среды

Не добавляйте весь 32-битный sysroot в хостовый `LD_LIBRARY_PATH`: это смешивает хостовый загрузчик с чужой glibc. Для QEMU используйте целевой `QEMU_LD_PREFIX`; дополнительные пути библиотек передавайте гостю, а не процессам хоста.

В проверенной среде QEMU/ARM32 операция `readdir` на хостовой ext4 давала `EOVERFLOW`, хотя inode помещались в 32 бита; та же неизменённая JVM корректно читала tmpfs и `/proc`. Это мешало в том числе чтению штатной JCE policy. Поэтому ARMv7 QA-AppDir и его изолированные данные размещались на tmpfs. Криптографические настройки, проверки TLS и файлы JVM не ослаблялись. Это ограничение эмуляционного теста, а не основание игнорировать ошибки чтения в приложении. Запуск на физическом ARMv7-устройстве требует отдельной проверки.

Проверки разных ABI, запускающие сетевые плагины, лучше выполнять последовательно: свободный порт, полученный через временный socket, не остаётся зарезервированным после закрытия socket.

## Упаковка

Общий упаковщик: `linux/package_appimage.py`; Debian/Arch: `linux/package_linux.py`. Они не запускают рабочий VPN и ничего не публикуют. Для 32-битного выпуска обязательны совместимые по ABI JRE, Skiko, JavaFX/WebKit, ядро, supervisor, трей и плагины. Указание только нейтрального `javafx.web.jar` недостаточно. Итоговый каталог и список артефактов нельзя считать проверенными до успешного `--webview-self-test` и повторной проверки упакованного приложения.


### Подготовка плагинов и JAR из SDK

`fetch_plugins.py` использует проверяемые SHA-256 из `plugins-sources.json`. Для Mieru x86 выполняется настоящий Go cross-build из закреплённого исходного архива, поскольку upstream не предоставляет готовый x86 asset.

```bash
python3 linux/port32/fetch_plugins.py --arch all
python3 linux/port32/prepare_runtime_jars.py --arch x86 \
  --fx-sdk "$NEKOBOX_PORT32_WORK/fx-i386-src/jfx21u-5497f2f46abff58bea405b310fb6c9670686709e/build/sdk" \
  --output-dir "$NEKOBOX_PORT32_WORK/jars-x86"
```

Для ARMv7 подставьте `--arch armv7`, `fx-armhf-src` и отдельный `jars-armv7`. `build_skiko.py` собирает также нейтральный `awtJar`; стандартный Skiko x64 не должен оставаться рядом с портированным JNI.

После общей JVM-сборки из корневой инструкции нативную часть собирают отдельно для каждого ABI. Пример x86:

```bash
CXX="$NEKOBOX_PORT32_WORK/graphics-i386/cxx" \
PKG_CONFIG="$NEKOBOX_PORT32_WORK/graphics-i386/pkg-config" \
GOARCH=386 GO386=softfloat bash linux/build-linux.sh --native-only
python3 linux/package_appimage.py --arch x86 \
  --jre "$NEKOBOX_PORT32_WORK/runtime-i386-complete" \
  --native-dir linux/bin --jars-dir "$NEKOBOX_PORT32_WORK/jars-x86" \
  --plugins-dir "$NEKOBOX_PORT32_WORK/plugins32/x86" \
  --extra-native-dir "$NEKOBOX_PORT32_WORK/fx-i386-src/jfx21u-5497f2f46abff58bea405b310fb6c9670686709e/build/sdk/lib" \
  --output-dir "$NEKOBOX_PORT32_WORK/gui-x86"
python3 linux/package_linux.py --arch x86 \
  --appdir "$NEKOBOX_PORT32_WORK/gui-x86/Nekobox-YTM-1.4.2.28-x86.AppDir" \
  --output-dir "$NEKOBOX_PORT32_WORK/packages-x86"
```

Для ARMv7: `GOARCH=arm GOARM=7`, обёртки `graphics-armhf`, целевой Full JRE из `runtime-armhf/`, JAR/плагины ARMv7 и SDK из `fx-armhf-src`. Сохраните `linux/bin` каждой архитектуры отдельно до следующего cross-build. Серверные пакеты собираются независимо и не требуют этих GUI-зависимостей.
