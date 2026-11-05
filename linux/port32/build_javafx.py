"""Cross-build полного SDK OpenJFX 21 с WebKit для x86 либо ARMv7.
SPDX-License-Identifier: GPL-3.0-or-later
Работает в отдельном каталоге, ничего не устанавливает и не запускает рабочий VPN.
"""
from pathlib import Path
import argparse, os, subprocess, tarfile, shutil, json, shlex
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--arch',choices=['i386','armhf'],required=True);p.add_argument('--jobs',type=int,default=1);a=p.parse_args();assert 1<=a.jobs<=8
repository=Path(__file__).resolve().parents[2];P=Path(os.environ['NEKOBOX_PORT32_WORK']).resolve();R=P/('sysroot-'+a.arch);H=Path(os.environ.get('NEKOBOX_FX_HOST_TOOLS',str(P/'fx-host-tools')));J=Path(os.environ['JAVA_HOME']).resolve()
C=P/('fx-'+a.arch+'-src');F=C/'jfx21u-5497f2f46abff58bea405b310fb6c9670686709e';W=P/('fx-tools-'+a.arch);C.mkdir(exist_ok=True);W.mkdir(exist_ok=True)
if not F.exists():
 with tarfile.open(P/'javafx-21.0.13.1-source.tar.gz')as archive:archive.extractall(C,filter='data')
 subprocess.run(['patch','-p1','-i',str(Path(__file__).parent/'patches/openjfx-32bit.patch')],cwd=F,check=True)
if a.arch=='armhf':
 f=F/'build.gradle';s=f.read_text().replace('OS_ARCH != "amd64" && !IS_AARCH64','OS_ARCH != "amd64" && OS_ARCH != "armv7l" && !IS_AARCH64').replace('cmakeArgs = "$cmakeArgs -DCMAKE_SYSTEM_PROCESSOR=i586"','cmakeArgs = "$cmakeArgs -DCMAKE_SYSTEM_PROCESSOR=armv7l"');f.write_text(s)
 f=F/'buildSrc/linux.gradle';f.write_text(f.read_text().replace('if (!IS_64) {','if (!IS_64 && OS_ARCH == "i386") {'))
for name,tool in [('gcc','cc'),('g++','cxx')]:
 host='clang'if tool=='cc'else'clang++';f=W/name;thumb=' -mthumb'if a.arch=='armhf'else''
 f.write_text('#!/bin/bash\n# Генераторы хоста не подменяются целевой архитектурой.\nfor arg in "$@"; do if [[ "$arg" == -m64 ]]; then exec /usr/bin/'+host+' "$@"; fi; done\nexec '+shlex.quote(str(P/('graphics-'+a.arch)/tool))+thumb+' "$@"\n');f.chmod(0o755)
for name,target in [('pkg-config',P/('graphics-'+a.arch)/'pkg-config'),('strip',Path(shutil.which('llvm-strip')or'/usr/bin/strip'))]:
 if not(W/name).exists():(W/name).symlink_to(target)
triple='i386-linux-gnu'if a.arch=='i386'else'arm-linux-gnueabihf';runtime=P/'runtime-i386-complete'if a.arch=='i386'else next((P/'runtime-armhf').glob('*/bin/java')).parent.parent
assert(runtime/'bin/java').is_file();env=os.environ.copy();env.update(JAVA_HOME=str(J),YTM_FX_ARCH='i386'if a.arch=='i386'else'armv7l',YTM_TARGET_JAVA_HOME=str(runtime),CC=str(W/'gcc'),CXX=str(W/'g++'),PKG_CONFIG=str(W/'pkg-config'),PKG_CONFIG_SYSROOT_DIR=str(R),PKG_CONFIG_LIBDIR=':'.join([str(R/('usr/lib/'+triple+'/pkgconfig')),str(R/'usr/share/pkgconfig')]))
if a.arch=='armhf':env.update(QEMU_LD_PREFIX=str(R),QEMU_CPU='cortex-a9')
paths=[str(W),str(J/'bin')]
if(H/'usr/bin/ruby').exists():
 # Необязательные изолированные инструменты; glibc целевого sysroot сюда не попадает.
 env['LD_LIBRARY_PATH']=str(H/'usr/lib');env['RUBYLIB']=':'.join([str(H/'usr/lib/ruby/3.4.0'),str(H/'usr/lib/ruby/3.4.0/x86_64-linux'),str(H/'usr/lib/ruby/vendor_ruby')]);env['GEM_HOME']=str(H/'gems');env['GEM_PATH']=str(H/'gems')+':'+str(H/'usr/lib/ruby/gems/3.4.0');paths.insert(1,str(H/'usr/bin'))
env['PATH']=':'.join(paths+[env['PATH']]);env['GRADLE_OPTS']='-Xmx1024m -Dorg.gradle.workers.max='+str(a.jobs)
subprocess.run(['ruby','-rerb','-rjson','-ryaml','-rgetoptlong','-e','puts "Зависимости Ruby готовы"'],env=env,check=True)
args=['bash',str(repository/'android/gradlew'),'--no-daemon','--console=plain','--max-workers='+str(a.jobs),'-PtoolchainDir='+str(W),'-PCOMPILE_TARGETS=linux','-PCOMPILE_WEBKIT=true','-PCOMPILE_MEDIA=true','-PNUM_COMPILE_THREADS='+str(a.jobs),'sdk']
log=P/('javafx-'+a.arch+'-public-build.log');print('Начата сборка',a.arch,'; журнал:',log,flush=True)
with log.open('w')as output:r=subprocess.run(args,cwd=F,env=env,stdout=output,stderr=subprocess.STDOUT)
sdk_lib=F/'build/sdk/lib';required_so=['libjfxwebkit.so','libjfxmedia.so','libgstreamer-lite.so','libfxplugins.so'];webkit=[sdk_lib/n for n in required_so if (sdk_lib/n).is_file()];ok=r.returncode==0 and len(webkit)==len(required_so)
(P/('javafx-'+a.arch+'-public-result.json')).write_text(json.dumps({'code':r.returncode,'source':str(F),'webkit':[str(f)for f in webkit],'sdk_built':ok},ensure_ascii=False,indent=2))
assert ok,'Полный SDK не собран; см. журнал. Неполный результат нельзя выдавать за релиз.'
print('SDK собран. Перед упаковкой обязательна проверка WebView в целевой JVM.',flush=True)
