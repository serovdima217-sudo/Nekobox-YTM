"""Получение закреплённых исходников и настоящих 32-битных JRE без системной установки."""
from pathlib import Path
import argparse, os, hashlib, urllib.request, urllib.parse, shutil, subprocess, tarfile, json, struct

def download(url, path, digest):
 if not path.exists():
  temporary=path.with_suffix(path.suffix+'.part')
  print('Получение:',path.name,flush=True)
  with urllib.request.urlopen(url,timeout=120)as source,temporary.open('wb')as target:shutil.copyfileobj(source,target)
  with temporary.open('rb')as source:actual=hashlib.file_digest(source,'sha256').hexdigest()
  if actual!=digest:temporary.unlink();raise RuntimeError('Не совпала SHA-256: '+path.name)
  temporary.replace(path)
 with path.open('rb')as source:assert hashlib.file_digest(source,'sha256').hexdigest()==digest,'Не совпала SHA-256: '+path.name
 return path

def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--verify-only',action='store_true',help='Только проверка уже полученных архивов');args=parser.parse_args()
 here=Path(__file__).resolve().parent;work=Path(os.environ['NEKOBOX_PORT32_WORK']).resolve();work.mkdir(parents=True,exist_ok=True)
 records=json.loads((here/'sources.json').read_text());names={'skiko':'skiko-source.tar.gz','skia':'skia-source.tar.gz','openjfx':'javafx-21.0.13.1-source.tar.gz'}
 for record in records:
  archive=work/names[record['component']]
  if args.verify_only:
   with archive.open('rb')as stream:assert hashlib.file_digest(stream,'sha256').hexdigest()==record['archive_sha256']
   continue
  download(record['url'],archive,record['archive_sha256']);prefix={'openjfx':'jfx21u'}.get(record['component'],record['component']);tree=work/(prefix+'-'+record['commit'])
  if not tree.exists():
   with tarfile.open(archive)as source:source.extractall(work,filter='data')
  patch=here/'patches'/(record['component']+'-32bit.patch')
  if patch.stat().st_size:
   command=['patch','-p1','-i',str(patch)];reverse=subprocess.run(command+['--dry-run','-R'],cwd=tree,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
   if reverse.returncode:subprocess.run(command,check=True,cwd=tree)
 if args.verify_only:
  print('SHA-256 трёх исходных архивов подтверждены.');return
 version='21.0.13.0.3+5';base='https://github.com/bell-sw/Liberica/releases/download/'+urllib.parse.quote(version,safe='')+'/'
 inputs=[('bellsoft-jdk'+version+'-linux-i586.tar.gz','7bdaa6b2892e4344002a077c07bb6147d29dc2571c358f4077bb35c40435cf99','jdk-i386'),('bellsoft-jre'+version+'-linux-arm32-vfp-hflt-full.tar.gz','8e41dd9ac68d15f509b9f4c6d640bb550c1d248983b190f05ed033fa5132ecdf','runtime-armhf')]
 for name,digest,destination in inputs:
  archive=download(base+urllib.parse.quote(name,safe=''),work/name,digest);target=work/destination;target.mkdir(exist_ok=True)
  if not list(target.glob('*/bin/java')):
   with tarfile.open(archive)as source:source.extractall(target,filter='data')
 target_jdk=next((work/'jdk-i386').glob('*/jmods')).parent;assert(target_jdk/'jmods/jdk.unsupported.desktop.jmod').is_file()
 runtime=work/'runtime-i386-complete'
 if not runtime.exists():
  modules='java.se,jdk.crypto.ec,jdk.unsupported,jdk.httpserver,jdk.zipfs,jdk.charsets,jdk.unsupported.desktop,jdk.jsobject,jdk.xml.dom'
  subprocess.run([str(Path(os.environ['JAVA_HOME'])/'bin/jlink'),'-J-Xmx512m','--module-path',str(target_jdk/'jmods'),'--add-modules',modules,'--strip-debug','--no-header-files','--no-man-pages','--compress=2','--output',str(runtime)],check=True)
 # Компоновщик хоста не заменяет целевые бинарники: проверяем класс и e_machine ELF.
 for executable,machine in [(runtime/'bin/java',3),(runtime/'lib/server/libjvm.so',3)]:
  with executable.open('rb')as source:header=source.read(20)
  assert header[:5]==b'\x7fELF\x01'and struct.unpack('<H',header[18:20])[0]==machine,'Получен не настоящий i386 ELF'
 directory=work/'runtime-i386';directory.mkdir(exist_ok=True);link=directory/'jre-complete'
 if not list(directory.glob('*/bin/java')):link.symlink_to(runtime,target_is_directory=True)
 print('Закреплённые исходники, патчи и настоящие 32-битные runtime подготовлены.')

if __name__=='__main__':main()
