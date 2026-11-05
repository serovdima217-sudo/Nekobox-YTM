"""Debian/Arch-пакеты из подготовленного и проверенного GUI AppDir.
SPDX-License-Identifier: GPL-3.0-or-later
Ничего не устанавливает, не запускает VPN и не меняет пользовательские данные.
"""
from pathlib import Path
import argparse, hashlib, json, os, re, shutil, subprocess, tarfile, tempfile, time, zipfile, struct
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--appdir',type=Path,required=True)
p.add_argument('--arch',choices=['x86_64','aarch64','x86','armv7'],required=True)
p.add_argument('--version',default='1.4.2.28')
p.add_argument('--output-dir',type=Path,required=True)
p.add_argument('--maintainer',default='Nekobox YTM contributors')
a=p.parse_args()
assert re.fullmatch(r'[0-9][0-9A-Za-z.+~]*',a.version)
assert '\n'not in a.maintainer and '\r'not in a.maintainer
assert(a.appdir/'AppRun').is_file()and(a.appdir/'usr/libexec/nekobox-core').is_file()
assert(a.appdir/'usr/share/licenses/nekobox-ytm').is_dir(),'Отсутствуют лицензии'
assert not a.output_dir.resolve().is_relative_to(a.appdir.resolve()), 'Выходной каталог не должен находиться внутри AppDir'
a.output_dir.mkdir(parents=True,exist_ok=True)
deb_arch,arch_arch={'x86_64':('amd64','x86_64'),'aarch64':('arm64','aarch64'),'x86':('i386','i686'),'armv7':('armhf','armv7h')}[a.arch]
versions={'GLIBC':set(),'GLIBCXX':set()}
def scan(f):
 with f.open('rb')as stream:
  head=stream.read(64)
  if head[:4]!=b'\x7fELF':return
  expected={'x86_64':(2,62),'aarch64':(2,183),'x86':(1,3),'armv7':(1,40)}[a.arch]
  assert head[4]==expected[0] and head[5]==1 and struct.unpack_from('<H',head,18)[0]==expected[1], 'Чужой ELF ABI в GUI: '+str(f)
 text=subprocess.check_output(['readelf','--version-info',str(f)],text=True,stderr=subprocess.DEVNULL)
 for kind,value in re.findall(r'Name: (GLIBCXX|GLIBC)_(\d[\d.]*)',text):versions[kind].add(value)
def root_owner(info):
 info.uid=info.gid=0;info.uname=info.gname='root';info.mode &= 0o1777
 if 'SOURCE_DATE_EPOCH'in os.environ:info.mtime=int(os.environ['SOURCE_DATE_EPOCH'])
 return info
def highest(values):return max(values,key=lambda x:tuple(map(int,x.split('.'))))if values else None
def sha(f):
 with f.open('rb')as stream:return hashlib.file_digest(stream,'sha256').hexdigest()
with tempfile.TemporaryDirectory(prefix='nekobox-package-',dir=a.output_dir)as temp:
 T=Path(temp);root=T/'root';app=root/'opt/nekobox-ytm';app.parent.mkdir(parents=True);shutil.copytree(a.appdir,app,symlinks=True)
 for f in app.rglob('*'):
  if f.is_file()and not f.is_symlink():scan(f)
 # В зависимости пакета входят и реально загружаемые JNI из JAR.
 counter=0
 for jar in(app/'usr/lib/nekobox/jars').glob('*.jar'):
  with zipfile.ZipFile(jar)as z:
   for entry in z.infolist():
    if entry.filename.endswith('.so'):
     with z.open(entry)as stream:head=stream.read(4)
     if head==b'\x7fELF':
      counter+=1;f=T/('jni-'+str(counter)+'.so');f.write_bytes(z.read(entry));scan(f);f.unlink()
 libc=highest(versions['GLIBC'])or'2.17';cpp=highest(versions['GLIBCXX'])
 # Минимальные версии GCC runtime согласно версии экспортируемого GLIBCXX ABI.
 gcc_map={'3.4.26':'9','3.4.27':'9.2','3.4.28':'10','3.4.29':'11','3.4.30':'12','3.4.31':'13.1','3.4.32':'13.2','3.4.33':'14','3.4.34':'15','3.4.35':'16'}
 gcc=gcc_map.get(cpp)
 if cpp and tuple(map(int,cpp.split('.')))>=(3,4,26):assert gcc,'Добавьте в таблицу новый GLIBCXX ABI: '+cpp
 wrapper=root/'usr/bin/nekobox-ytm';wrapper.parent.mkdir(parents=True);wrapper.write_text('#!/bin/sh\nexec /opt/nekobox-ytm/AppRun "$@"\n');wrapper.chmod(0o755)
 desktop=root/'usr/share/applications/nekobox-ytm.desktop';desktop.parent.mkdir(parents=True);desktop.write_text('[Desktop Entry]\nType=Application\nName=Nekobox YTM\nComment=Нативный Linux-порт NekoBox for Android\nExec=nekobox-ytm %U\nIcon=nekobox-ytm\nTerminal=false\nCategories=Network;\nStartupWMClass=io-nekohasekai-sagernet-linux-MainKt\n')
 icon=root/'usr/share/icons/hicolor/512x512/apps/nekobox-ytm.png';icon.parent.mkdir(parents=True);shutil.copy2(app/'nekobox-ytm.png',icon)
 notices=root/'usr/share/licenses/nekobox-ytm';notices.parent.mkdir(parents=True,exist_ok=True);shutil.copytree(app/'usr/share/licenses/nekobox-ytm',notices)
 dependencies=['libc6 (>= '+libc+')','libstdc++6'+(' (>= '+gcc+')'if gcc else ''),'libgcc-s1','libatomic1','libegl1','libx11-6','libxext6','libxrender1','libxtst6','libxi6','libxrandr2','libxxf86vm1','libfontconfig1','libfreetype6','libasound2 | libasound2t64','libdbus-1-3','libgl1','libgtk-3-0 | libgtk-3-0t64','pkexec']
 control=T/'control';control.mkdir();size=sum(f.stat().st_size for f in root.rglob('*')if f.is_file())//1024
 (control/'control').write_text('Package: nekobox-ytm\nVersion: '+a.version+'-1\nArchitecture: '+deb_arch+'\nMaintainer: '+a.maintainer+'\nSection: net\nPriority: optional\nInstalled-Size: '+str(size)+'\nDepends: '+', '.join(dependencies)+'\nDescription: Nekobox YTM — графический VPN-клиент\n Полный JVM/Linux GUI с нативным ядром; пользовательское состояние сохраняется.\n')
 for name in['postinst','postrm']:
  f=control/name;f.write_text('#!/bin/sh\nset -e\nif command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database -q /usr/share/applications || true; fi\nif command -v gtk-update-icon-cache >/dev/null 2>&1; then gtk-update-icon-cache -q -t /usr/share/icons/hicolor || true; fi\n# Приложение и VPN не запускаются; пользовательские файлы не удаляются.\nexit 0\n');f.chmod(0o755)
 for source,name in[(control,'control.tar.gz'),(root,'data.tar.gz')]:
  with tarfile.open(T/name,'w:gz')as tar:
   for f in sorted(source.iterdir()):tar.add(f,arcname='./'+f.name,filter=root_owner)
 (T/'debian-binary').write_text('2.0\n');deb=a.output_dir/('nekobox-ytm_'+a.version+'-1_'+deb_arch+'.deb');assert not deb.exists()
 subprocess.run(['ar','rc',str(deb.resolve()),'debian-binary','control.tar.gz','data.tar.gz'],cwd=T,check=True)
 arch_deps=['glibc>='+libc,'gcc-libs'+('>='+gcc if gcc else ''),'libx11','libxext','libxrender','libxtst','libxi','libxrandr','libxxf86vm','fontconfig','freetype2','alsa-lib','dbus','libglvnd','gtk3','polkit']
 pkginfo=T/'.PKGINFO';pkginfo.write_text('pkgname = nekobox-ytm\npkgbase = nekobox-ytm\npkgver = '+a.version+'-1\npkgdesc = Nekobox YTM — графический VPN-клиент\nbuilddate = '+str(int(os.environ.get('SOURCE_DATE_EPOCH',time.time())))+'\npackager = '+a.maintainer+'\nsize = '+str(size*1024)+'\narch = '+arch_arch+'\nlicense = GPL-3.0-or-later\n'+''.join('depend = '+d+'\n'for d in arch_deps))
 install=T/'.INSTALL';install.write_text('post_install() {\n  command -v update-desktop-database >/dev/null && update-desktop-database -q /usr/share/applications || true\n  command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -q -t /usr/share/icons/hicolor || true\n}\npost_upgrade() { post_install; }\npost_remove() { post_install; }\n# Не запускать VPN и не удалять пользовательские данные.\n')
 raw=T/'package.tar'
 with tarfile.open(raw,'w')as tar:
  tar.add(pkginfo,arcname='.PKGINFO',filter=root_owner);tar.add(install,arcname='.INSTALL',filter=root_owner)
  for f in sorted(root.iterdir()):tar.add(f,arcname=f.name,filter=root_owner)
 arch=a.output_dir/('nekobox-ytm-'+a.version+'-1-'+arch_arch+'.pkg.tar.zst');assert not arch.exists();subprocess.run(['zstd','-q','-T2','-15',str(raw),'-o',str(arch)],check=True)
 artifacts=[{'name':f.name,'bytes':f.stat().st_size,'sha256':sha(f)}for f in[deb,arch]]
 result={'arch':a.arch,'artifacts':artifacts,'GLIBC':libc,'GLIBCXX':cpp,'installed':False,'vpn_restarted':False}
 (a.output_dir/('packages-'+a.arch+'.json')).write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result,ensure_ascii=False))
