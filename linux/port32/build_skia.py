"""Изолированный настоящий cross-build графики x86/ARMv7, без подмены 64-битными библиотеками."""
from pathlib import Path
import os,subprocess,json,shlex,tarfile,time,traceback
P=Path(os.environ['NEKOBOX_PORT32_WORK']).resolve();S=P/'skia-22f58c9fd43d55bde818821c04b48fda5d7ec939';records=[]
def run(name,args,cwd=S,env=None,timeout=1800):
 print('Начато:',name,flush=True)
 with(P/(name+'.log')).open('w')as f:r=subprocess.run(args,cwd=cwd,env=env,stdout=f,stderr=subprocess.STDOUT,timeout=timeout)
 records.append({'name':name,'code':r.returncode});(P/'graphics32-results.json').write_text(json.dumps(records,ensure_ascii=False,indent=2));print('Результат:',name,r.returncode,flush=True)
 if r.returncode:raise RuntimeError('Не пройден этап: '+name)
try:
 for arch,triple,target,cpu,qemu,archive in [('i386','i386-linux-gnu','i686-linux-gnu','x86','qemu-i386-static','bellsoft-jre21.0.13.0.3+5-linux-i586.tar.gz'),('armhf','arm-linux-gnueabihf','arm-linux-gnueabihf','arm','qemu-arm-static','bellsoft-jre21.0.13.0.3+5-linux-arm32-vfp-hflt-full.tar.gz')]:
  R=P/('sysroot-'+arch);W=P/('graphics-'+arch);W.mkdir(exist_ok=True);gcc=next(R.glob('usr/lib/gcc/*/12'))
  base=['--target='+target,'--sysroot='+str(R),'--gcc-install-dir='+str(gcc)]
  if arch=='armhf':base+=['-march=armv7-a','-mfpu=vfpv3-d16','-mfloat-abi=hard']
  for name,tool in [('cc','clang'),('cxx','clang++')]:
   flags=base.copy()
   if name=='cxx':
    for include in [R/'usr/include/c++/12',R/('usr/include/'+triple+'/c++/12'),R/'usr/include/c++/12/backward']:flags+=['-isystem',str(include)]
   f=W/name;f.write_text('#!/bin/bash\n# Хостовые генераторы остаются 64-битными; целевая графика — строго 32-битная.\nfor arg in "$@"; do if [[ "$arg" == -m64 ]]; then exec /usr/bin/'+tool+' "$@"; fi; done\nexec /usr/bin/'+tool+' '+shlex.join(flags)+' "$@"\n');f.chmod(0o755)
   wrapper=f.read_text();marker='exec /usr/bin/'+tool+' '+shlex.join(flags)
   linkflags=['-fuse-ld=lld','-Wl,--undefined-version','-Wl,-rpath-link,'+str(R/'lib'/triple),'-Wl,-rpath-link,'+str(R/'usr/lib'/triple)]
   wrapper=wrapper.replace(marker,'linkflags=('+shlex.join(linkflags)+')\nfor arg in "$@"; do if [[ "$arg" == -c || "$arg" == -E ]]; then linkflags=(); fi; done\n'+marker+' "${linkflags[@]}"');f.write_text(wrapper)
  pc=W/'pkg-config';pc.write_text('#!/bin/sh\nexport PKG_CONFIG_SYSROOT_DIR='+shlex.quote(str(R))+'\nexport PKG_CONFIG_LIBDIR='+shlex.quote(':'.join(str(R/x)for x in ['usr/lib/'+triple+'/pkgconfig','usr/lib/pkgconfig','usr/share/pkgconfig']))+'\nexec /usr/bin/pkg-config "$@"\n');pc.chmod(0o755)
  probe=W/'probe.cpp';probe.write_text('#include <cstdio>\n#include <zlib.h>\n#include <fontconfig/fontconfig.h>\n#include <jni.h>\nint main(){static_assert(sizeof(void*)==4);static_assert(sizeof(jlong)==8);static_assert(sizeof(jobject)==4);std::printf("32-bit JNI; zlib=%s; fontconfig=%d\\n",zlibVersion(),FcInit());}\n')
  jdk=Path(os.environ['JAVA_HOME']).resolve();run('probe-build-'+arch,[str(W/'cxx'),str(probe),'-I'+str(jdk/'include'),'-I'+str(jdk/'include/linux'),'-lfontconfig','-lz','-o',str(W/'probe')])
  env=os.environ.copy();env.pop('LD_LIBRARY_PATH',None);env['QEMU_LD_PREFIX']=str(R);env['XDG_CACHE_HOME']=str(W/'cache')
  run('probe-run-'+arch,[qemu,'-E','LD_LIBRARY_PATH='+':'.join(str(R/x)for x in ['lib/'+triple,'usr/lib/'+triple]),'-L',str(R),str(W/'probe')],env=env)
  J=P/('runtime-'+arch);J.mkdir(exist_ok=True)
  if not list(J.glob('*/bin/java')):
   print('Распаковка проверенного JRE:',arch,flush=True)
   with tarfile.open(P/archive)as tf:tf.extractall(J,filter='data')
  java=next(J.glob('*/bin/java'));run('java-modules-'+arch,[qemu,'-E','LD_LIBRARY_PATH='+':'.join(str(R/x)for x in ['lib/'+triple,'usr/lib/'+triple]),'-L',str(R),str(java),'-Xint','--list-modules'],env=env,timeout=180)
  flags=['-USK_HIDE_PATH_EDIT_METHODS'];ccflags=['-fno-exceptions','-fno-rtti','-D_GLIBCXX_USE_CXX11_ABI=0','-USK_HIDE_PATH_EDIT_METHODS']
  args={'is_debug':False,'is_official_build':True,'is_clang':True,'target_os':'linux','target_cpu':cpu,'cc':str(W/'cc'),'cxx':str(W/'cxx'),'ar':'/usr/bin/llvm-ar','pkg_config':str(pc),'is_trivial_abi':False,'skia_enable_tools':False,'skia_use_system_expat':False,'skia_use_system_libjpeg_turbo':False,'skia_use_system_libpng':False,'skia_use_system_libwebp':False,'skia_use_system_zlib':False,'skia_use_sfntly':False,'skia_use_system_freetype2':False,'skia_use_system_harfbuzz':False,'skia_pdf_subset_harfbuzz':True,'skia_use_system_icu':False,'skia_enable_skottie':True,'skia_use_vulkan':False,'skia_use_dawn':False,'skia_use_gl':True,'skia_use_egl':True,'extra_cflags':flags,'extra_cflags_cc':ccflags}
  if arch=='armhf':args.update(arm_version=7,arm_use_neon=False,arm_float_abi='hard',skia_gl_standard='gles')
  out=S/'out'/('Release-linux-'+('x86'if arch=='i386'else'arm32'));out.mkdir(parents=True,exist_ok=True)
  text='\n'.join(k+' = '+json.dumps(v)for k,v in args.items());(out/'args.gn').write_text(text+'\n')
  run('gn-'+arch,[str(S/'bin/gn'),'gen',str(out)])
  run('skia-'+arch,['ninja','-j','2','-C',str(out),'skia','modules'],timeout=14400)
 print('Архивы Skia собраны; JNI Skiko и полноценный GUI проверяются отдельно.',flush=True)
except Exception:
 (P/'graphics32-error.txt').write_text(traceback.format_exc());print(traceback.format_exc(),flush=True)
 raise
