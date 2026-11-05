"""Пакетирование GUI без путей к рабочему ПК и без публикации/перезапуска приложения.
SPDX-License-Identifier: GPL-3.0-or-later
Зависимости целевой архитектуры готовятся отдельно; смешение ABI запрещено.
"""
from pathlib import Path
import argparse, hashlib, json, os, platform, shutil, struct, subprocess, zipfile
P = Path(__file__).resolve().parent
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--arch', choices=['x86_64','aarch64','x86','armv7'], required=True)
p.add_argument('--jre', type=Path, required=True)
p.add_argument('--runtime', type=Path, help='ELF runtime type2 AppImage целевой архитектуры')
p.add_argument('--runtime-license', type=Path, help='Лицензия выбранного runtime AppImage')
p.add_argument('--native-dir', type=Path, default=P/'bin')
p.add_argument('--jars-dir', type=Path, default=P/'runtime-jars')
p.add_argument('--application-jar', type=Path, default=P/'build/libs/Nekobox-YTM-Android-Linux.jar')
p.add_argument('--plugins-dir', type=Path, default=P/'plugins')
p.add_argument('--extra-native-dir', type=Path, help='Нативные библиотеки целевого JavaFX SDK')
p.add_argument('--output-dir', type=Path, default=P/'packages')
p.add_argument('--version', default='1.4.2.28')
p.add_argument('--no-publish', action='store_true', help='Совместимость; публикация всегда отключена')
a = p.parse_args()
elf_class, machine = {'x86_64':(2,62),'aarch64':(2,183),'x86':(1,3),'armv7':(1,40)}[a.arch]

def elf(data, name):
    if data[:4] != b'\x7fELF': return False
    assert data[4] == elf_class and data[5] == 1 and struct.unpack('<H',data[18:20])[0] == machine, 'Чужой ELF ABI: '+str(name)
    return True

def digest(f):
    with f.open('rb') as stream: return hashlib.file_digest(stream,'sha256').hexdigest()

assert (a.jre/'bin/java').is_file(), 'Не найден целевой JRE'
release = (a.jre/'release').read_text()
import re
major = int(re.search(r'JAVA_VERSION="(\d+)', release)[1])
assert major >= 21, 'Требуется JRE 21 или новее'
assert 'jdk.unsupported.desktop' in release, 'JRE не содержит модуль Swing interop; используйте полный целевой JRE/JDK jmods'
assert not a.output_dir.resolve().is_relative_to(a.jre.resolve()), 'Выходной каталог нельзя помещать внутрь исходного JRE'
if a.arch in ['x86','armv7']:
    webkit = [a.jre/'lib/libjfxwebkit.so']
    if a.extra_native_dir: webkit.append(a.extra_native_dir/'libjfxwebkit.so')
    assert any(f.is_file() for f in webkit), 'Нет настоящего целевого libjfxwebkit.so: неполный 32-битный GUI не упаковывается в релиз'
    for name in ['libjfxmedia.so','libgstreamer-lite.so','libfxplugins.so']:
        media = [a.jre/'lib'/name]
        if a.extra_native_dir: media.append(a.extra_native_dir/name)
        assert any(f.is_file() for f in media), 'Неполный Media runtime для WebView: '+name

for name in ['nekobox-core','ytm-client','linux-supervisor','linux-tray']:
    f = a.native_dir/name
    assert f.is_file() and elf(f.read_bytes()[:64],f), 'Нет целевого нативного компонента: '+name
assert a.application_jar.is_file(), 'Сначала соберите JVM-часть'
jars = sorted(a.jars_dir.glob('*.jar')) + [a.application_jar]
assert any(f.name.startswith('skiko') for f in jars), 'Не подготовлен Skiko'
assert any(f.name.startswith('javafx-web') or f.name == 'javafx.web.jar' for f in jars), 'Не подготовлен JavaFX WebKit'
pruned = {}
# Проверяется не только имя JAR: внутри не должно быть чужих ELF или слишком новых классов.
for jar in jars:
    with zipfile.ZipFile(jar) as archive:
        seen_native = matched_native = 0
        for entry in archive.infolist():
            if entry.is_dir(): continue
            if entry.filename.endswith(('.so','.class')):
                with archive.open(entry) as stream: head = stream.read(64)
                if head[:4] == b'\x7fELF':
                    seen_native += 1
                    matches = head[4] == elf_class and head[5] == 1 and struct.unpack('<H',head[18:20])[0] == machine
                    if a.arch == 'armv7' and matches:
                        matches = not (struct.unpack('<I',head[36:40])[0] & 0x200)
                    if not matches:
                        assert jar.name.startswith(('sqlite-jdbc-','jna-')), 'Чужой ELF ABI: '+jar.name+':'+entry.filename
                        pruned.setdefault(str(jar),set()).add(entry.filename)
                        continue
                    matched_native += 1
                    elf(head,str(jar.name)+':'+entry.filename)
                if head[:4] == b'\xca\xfe\xba\xbe':
                    assert struct.unpack('>H',head[6:8])[0] <= major+44, 'Класс требует более нового JRE: '+str(jar)
        assert seen_native == 0 or matched_native > 0, 'В JAR нет нативной библиотеки целевого ABI: '+jar.name
for f in a.jre.rglob('*'):
    if f.is_file() and not f.is_symlink():
        with f.open('rb') as stream: elf(stream.read(64),f)
if a.runtime:
    assert a.runtime_license and a.runtime_license.is_file(), 'Передайте --runtime-license: исходные notices runtime должны сохраняться'
    with a.runtime.open('rb') as stream: assert elf(stream.read(64),a.runtime)
a.output_dir.mkdir(parents=True, exist_ok=True)
D = a.output_dir/('Nekobox-YTM-'+a.version+'-'+a.arch+'.AppDir')
assert not D.exists(), 'Каталог уже существует; выберите новый --output-dir, ничего не удаляется автоматически'
(D/'usr/lib/nekobox/jars').mkdir(parents=True)
(D/'usr/libexec').mkdir(parents=True)
for jar in jars:
    dest=D/'usr/lib/nekobox/jars'/jar.name
    if str(jar) not in pruned:
        shutil.copy2(jar,dest)
    else:
        # SQLite/JNA исходно универсальны; оставляем нужные ELF, Java-код и все лицензии.
        with zipfile.ZipFile(jar) as source, zipfile.ZipFile(dest,'w',compression=zipfile.ZIP_DEFLATED) as target:
            assert not any(n.upper().endswith(('.SF','.RSA','.DSA')) for n in source.namelist()), 'Подписанный JAR нельзя перепаковывать'
            for entry in source.infolist():
                if entry.filename not in pruned[str(jar)]: target.writestr(entry,source.read(entry.filename))
for name in ['nekobox-core','ytm-client','linux-supervisor','linux-tray']:
    shutil.copy2(a.native_dir/name,D/'usr/libexec'/name)
for f in sorted(a.plugins_dir.glob('*')):
    if f.is_file() and f.suffix != '.json':
        assert f.name not in ['nekobox-core','ytm-client','linux-supervisor','linux-tray'], 'Каталог плагинов не должен подменять свежие обязательные компоненты'
        with f.open('rb') as stream: elf(stream.read(64),f)
        shutil.copy2(f,D/'usr/libexec'/f.name)
shutil.copytree(a.jre,D/'usr/lib/nekobox/jre',symlinks=True)
if a.extra_native_dir:
    for f in a.extra_native_dir.glob('*.so'):
        with f.open('rb') as stream: assert elf(stream.read(64),f)
        shutil.copy2(f,D/'usr/lib/nekobox/jre/lib'/f.name)
L = D/'usr/share/licenses/nekobox-ytm'; L.mkdir(parents=True)
if a.runtime_license: shutil.copy2(a.runtime_license,L/'AppImage-runtime-LICENSE.txt')
repository = P.parent
for base in [repository, repository/'android', repository/'shared/sing-box',repository/'shared/telemost',repository/'LICENSES',P/'native/third-party-notices']:
    if not base.exists(): continue
    entries = base.rglob('*') if base.name in ['LICENSES','third-party-notices'] else base.iterdir()
    for f in entries:
        if f.is_file() and (base.name in ['LICENSES','third-party-notices'] or f.name.lower().startswith(('license','copying','copyright','notice','authors','third_party'))):
            dest = L/base.name/f.relative_to(base); dest.parent.mkdir(parents=True,exist_ok=True); shutil.copy2(f,dest)
for jar in jars:
    with zipfile.ZipFile(jar) as archive:
        for entry in archive.infolist():
            n = Path(entry.filename)
            if not entry.is_dir() and not n.is_absolute() and '..' not in n.parts and any(v in entry.filename.lower() for v in ['license','notice','legal/']):
                dest=L/'JAR'/jar.name/n;dest.parent.mkdir(parents=True,exist_ok=True);dest.write_bytes(archive.read(entry))
for source,name in [(P/'src/main/resources/android/assets/openjfx-licenses','OpenJFX'),(P/'src/main/resources/android/fonts','Roboto')]:
    if source.exists():shutil.copytree(source,L/name,ignore=shutil.ignore_patterns('*.ttf'))
shutil.copy2(P/'src/main/resources/android/icon/nekobox.png',D/'nekobox-ytm.png')
(D/'.DirIcon').symlink_to('nekobox-ytm.png')
(D/'nekobox-ytm.desktop').write_text('[Desktop Entry]\nType=Application\nName=Nekobox YTM\nComment=Нативный Linux-порт NekoBox for Android\nExec=AppRun %U\nIcon=nekobox-ytm\nTerminal=false\nCategories=Network;\nStartupWMClass=io-nekohasekai-sagernet-linux-MainKt\n')
flags = '--enable-native-access=ALL-UNNAMED' + (' --sun-misc-unsafe-memory-access=allow' if major>=25 else '')
if a.arch in ['x86','armv7']: flags += ' -Dskiko.renderApi=SOFTWARE'
# В Full JRE могут быть старые JavaFX-модули: используем именно поставленные SDK JAR.
flags += ' --limit-modules=java.se,jdk.crypto.ec,jdk.unsupported,jdk.httpserver,jdk.zipfs,jdk.charsets,jdk.unsupported.desktop,jdk.jsobject,jdk.xml.dom'
launcher = '''#!/bin/bash
set -euo pipefail
umask 077
HERE="$(cd -- "$(dirname -- "$0")" && pwd)"
export APPDIR="$HERE"
ROOT="${XDG_CACHE_HOME:-$HOME/.cache}/nekobox-ytm-android-linux"
mkdir -p "$ROOT/native" "$ROOT/java-tmp"
HASH="$(cat "$HERE"/usr/libexec/* | sha256sum | cut -c1-24)"
CACHE="$ROOT/native/$HASH"
if [[ ! -x "$CACHE/nekobox-core" || ! -x "$CACHE/linux-supervisor" || ! -x "$CACHE/ytm-client" || ! -x "$CACHE/linux-tray" ]]; then
 TMP="$(mktemp -d "$ROOT/native/.подготовка-XXXXXX")"
 cp "$HERE"/usr/libexec/* "$TMP/"
 chmod 755 "$TMP"/*
 if ! mv -T "$TMP" "$CACHE" 2>/dev/null; then rm -rf -- "$TMP"; fi
fi
export NEKOBOX_YTM_LIBEXEC="$CACHE"
CLASSPATH=""
for jar in "$HERE"/usr/lib/nekobox/jars/*.jar; do
 [[ -f "$jar" ]] || continue
 CLASSPATH="${CLASSPATH:+$CLASSPATH:}$jar"
done
exec "$HERE/usr/lib/nekobox/jre/bin/java" @FLAGS@ -Xms32m -Xmx768m -Dfile.encoding=UTF-8 -Dprism.order=sw -Dcompose.interop.blending=true -Djava.io.tmpdir="$ROOT/java-tmp" -cp "$CLASSPATH" io.nekohasekai.sagernet.linux.MainKt "$@"
'''
(D/'AppRun').write_text(launcher.replace('@FLAGS@',flags));(D/'AppRun').chmod(0o755)
artifacts=[]
if a.runtime:
    squash=a.output_dir/(D.stem+'.squashfs')
    subprocess.run(['mksquashfs',str(D),str(squash),'-noappend','-comp','zstd','-Xcompression-level','15','-processors','2','-no-progress'],check=True)
    out=a.output_dir/(D.stem+'.AppImage');assert not out.exists()
    with out.open('xb')as target:
        for f in [a.runtime,squash]:
            with f.open('rb')as source:shutil.copyfileobj(source,target)
    out.chmod(0o755);artifacts.append({'name':out.name,'bytes':out.stat().st_size,'sha256':digest(out)})
manifest={'version':a.version,'arch':a.arch,'jre_major':major,'appdir':str(D),'artifacts':artifacts,'pruned_foreign_elf':{Path(k).name:sorted(v)for k,v in pruned.items()},'status':'Создано; установка, запуск VPN и публикация не выполнялись. Требуются проверки GUI.'}
(a.output_dir/(D.stem+'-manifest.json')).write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')
print('Создан каталог GUI:',D)
for item in artifacts: print('Создан AppImage:',item['name'],'SHA256',item['sha256'])
