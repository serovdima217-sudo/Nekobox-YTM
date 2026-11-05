"""Сборка настоящих JNI Skiko x86/ARMv7 после соответствующих Skia-архивов."""
from pathlib import Path
import os,subprocess,json,select,shlex,time,traceback,struct
P=Path(os.environ['NEKOBOX_PORT32_WORK']).resolve();S=P/'skiko-9a5b398bb2044fff7e7a84fbfd6f4b803e4427c0/skiko';SKIA=P/'skia-22f58c9fd43d55bde818821c04b48fda5d7ec939';records=[]
try:
 env=os.environ.copy();env['JAVA_HOME']=os.environ['JAVA_HOME'];env['SKIA_DIR']=str(SKIA);env['PATH']=env['JAVA_HOME']+'/bin:'+env['PATH'];env['SKIKO_CXX_X86']=str(P/'graphics-i386/cxx');env['SKIKO_CXX_ARM32']=str(P/'graphics-armhf/cxx')
 for arch,root,machine in [('x86','i386',3),('arm32','armhf',40)]:
  if arch=='arm32':
   steps=json.loads((P/'graphics32-results.json').read_text());assert any(x['name']=='skia-armhf'and x['code']==0 for x in steps)
  env['PATH']=str(P/('graphics-'+root))+':'+env['JAVA_HOME']+'/bin:'+os.environ['PATH']
  title='X86'if arch=='x86'else'Arm32';args=['bash','../gradlew','--no-daemon','--console=plain','--max-workers=2','-Dorg.gradle.jvmargs=-Xmx768m -XX:MaxMetaspaceSize=1024m','-Pskiko.arch='+arch,'-Pdeploy.version=0.144.6-ytm32.1','awtJar','skikoJvmRuntimeJarLinux'+title]
  print('Начато JNI:',arch,flush=True)
  with(P/('skiko-jni-'+arch+'.log')).open('w')as f:r=subprocess.run(args,cwd=S,env=env,stdout=f,stderr=subprocess.STDOUT,timeout=3600)
  record={'arch':arch,'code':r.returncode};records.append(record);(P/'skiko-jni-results.json').write_text(json.dumps(records,indent=2));print('Результат JNI:',arch,r.returncode,flush=True);assert r.returncode==0
  libs=list((S/'build').rglob('libskiko-linux-'+arch+'.so'));assert libs
  for lib in libs:
   raw=lib.read_bytes()[:20];assert raw[:4]==b'\x7fELF'and raw[4]==1 and struct.unpack_from('<H',raw,18)[0]==machine,(lib,raw)
  record['elf32_machine']=machine;record['libraries']=[str(x)for x in libs];(P/'skiko-jni-results.json').write_text(json.dumps(records,indent=2))
 print('JNI собраны; проверка загрузки в 32-битной JVM и полноэкранного GUI ещё обязательна.',flush=True)
except Exception:
 (P/'skiko-jni-error.txt').write_text(traceback.format_exc());print(traceback.format_exc(),flush=True)
 raise
