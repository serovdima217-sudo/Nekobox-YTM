"""Изолированные библиотеки Debian i386/ARMv7 для линковки и проверок в QEMU, без установки в систему."""
from pathlib import Path
import urllib.request,lzma,json,hashlib,subprocess,concurrent.futures,shutil,re,time
import sys,os
arch=sys.argv[1];assert arch in ['armhf','i386']
B=Path(os.environ['NEKOBOX_PORT32_WORK']).resolve();D=B/('debian-'+arch);D.mkdir(parents=True,exist_ok=True);R=B/('sysroot-'+arch);R.mkdir(exist_ok=True)
index=D/'Packages.xz'
if not index.exists():
 with urllib.request.urlopen('https://deb.debian.org/debian/dists/bookworm/main/binary-'+arch+'/Packages.xz',timeout=90)as src,index.open('wb')as dst:shutil.copyfileobj(src,dst)
packages={};providers={}
with lzma.open(index,'rt')as src:
 fields={}
 for line in src:
  if line=='\n':
   if 'Package'in fields:
    packages[fields['Package']]=fields
    for virtual in fields.get('Provides','').split(','):
     if virtual.strip():providers.setdefault(virtual.strip().split()[0],fields['Package'])
   fields={};continue
  if line[0].isspace():continue
  k,_,v=line.partition(': ')
  if k in ['Package','Version','Filename','SHA256','Depends','Pre-Depends','Provides']:fields[k]=v.strip()
required=['libc6','libc6-dev','libstdc++6','libstdc++-12-dev','libgcc-s1','libdbus-1-3','libdbus-1-dev','libgtk-3-0','libxtst6','libxrender1','libxi6','libxext6','libxrandr2','libgl1','libfontconfig1','libfreetype6','libasound2','libasound2-dev','libavcodec-dev','libavformat-dev','libavutil-dev','libswresample-dev','libswscale-dev','libx11-6','libxkbcommon0','libxxf86vm1','libxxf86vm-dev','libharfbuzz0b','xvfb']
required += ['libgcc-12-dev','libatomic1','libfontconfig-dev','libfreetype-dev','libgl-dev','libegl-dev','libx11-dev','libxext-dev','libxrender-dev','libxtst-dev','libxi-dev','libxrandr-dev','libxkbcommon-dev','libgtk-3-dev','libwebkit2gtk-4.1-dev']
selected={};pending=required[:]
while pending:
 name=pending.pop()
 if name in selected:continue
 if name not in packages:name=providers[name]
 if name in selected:continue
 p=packages[name];selected[name]=p
 for field in ['Pre-Depends','Depends']:
  for group in p.get(field,'').split(','):
   if not group.strip():continue
   alternatives=[re.split(r'[\s(:]',x.strip(),maxsplit=1)[0]for x in group.split('|')]
   chosen=next((a for a in alternatives if a in packages or a in providers),None)
   assert chosen,('Не найдена библиотека',name,group)
   pending.append(chosen)
print('Изолированное окружение '+arch+':',len(selected),'пакетов.',flush=True)
def download(p):
 f=D/Path(p['Filename']).name
 if not f.exists():
  tmp=f.with_suffix('.часть')
  for attempt in range(4):
   try:
    with urllib.request.urlopen('https://deb.debian.org/debian/'+p['Filename'],timeout=90)as src,tmp.open('wb')as dst:shutil.copyfileobj(src,dst,1024*1024)
    tmp.rename(f);break
   except Exception:
    tmp.unlink(missing_ok=True)
    if attempt==3:raise
    time.sleep(2)
 assert hashlib.file_digest(f.open('rb'),'sha256').hexdigest()==p['SHA256'],f.name
 return f
files=list(concurrent.futures.ThreadPoolExecutor(max_workers=2).map(download,selected.values()))
for f in files:
 contents=subprocess.check_output(['bsdtar','-tf',str(f)],text=True).splitlines();name=next(n for n in contents if n.startswith('data.tar'))
 archive=subprocess.Popen(['bsdtar','-xOf',str(f),name],stdout=subprocess.PIPE)
 subprocess.run(['bsdtar','-xf','-','-C',str(R),'--no-same-owner'],stdin=archive.stdout,check=True);archive.stdout.close();assert archive.wait()==0
# Абсолютные ссылки Debian вне chroot иначе ведут на библиотеки хоста или на static libm.a.
import os
fixed_links=[]
for link in R.rglob('*'):
 if link.is_symlink():
  target=os.readlink(link)
  if target.startswith('/'):
   relative=os.path.relpath(R/target.lstrip('/'),link.parent)
   link.unlink();link.symlink_to(relative)
   fixed_links.append({'path':str(link.relative_to(R)),'old':target,'new':relative})
(B/('sysroot-'+arch+'-relative-links.json')).write_text(json.dumps(fixed_links,ensure_ascii=False,indent=2))
(B/('sysroot-'+arch+'-packages.json')).write_text(json.dumps(selected,ensure_ascii=False,indent=2))
print('Окружение '+arch+' готово:',R,flush=True)
