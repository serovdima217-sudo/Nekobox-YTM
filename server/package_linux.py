"""Серверные Debian/Arch-пакеты и отдельный бинарник без установки и запуска.
SPDX-License-Identifier: GPL-3.0-or-later
"""
from pathlib import Path
import argparse,shutil,os,io,json,hashlib,tarfile,gzip,subprocess,shutil,time,struct,re

p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--arch',required=True,choices=['x86_64','aarch64','armv7','x86'])
p.add_argument('--binary',required=True,type=Path,help='Статический сервер из shared/telemost/headless/telemost')
p.add_argument('--output-dir',required=True,type=Path)
p.add_argument('--version',default='1.4.2.28')
a=p.parse_args();R=Path(__file__).resolve().parents[1];O=a.output_dir.resolve();O.mkdir(parents=True,exist_ok=True)
assert re.fullmatch(r'[0-9]+(?:\.[0-9]+)*',a.version),'Версия должна состоять из чисел через точку'
V=a.version;stamp=int(os.environ.get('SOURCE_DATE_EPOCH',str(int(time.time()))));assert stamp>=0
arch=a.arch;debarch,pacarch,elfclass,machine={'x86_64':('amd64','x86_64',2,62),'aarch64':('arm64','aarch64',2,183),'armv7':('armhf','armv7h',1,40),'x86':('i386','i686',1,3)}[arch]
binary=a.binary.resolve();raw=binary.read_bytes()
assert raw[:6]==b'\x7fELF'+bytes([elfclass,1]) and struct.unpack_from('<H',raw,18)[0]==machine,'Чужой ELF ABI'
headers=subprocess.check_output(['readelf','-lW',str(binary)],text=True)
assert not re.search(r'^\s*(INTERP|DYNAMIC)\s',headers,re.M),'Нужен статический CGO_ENABLED=0 сервер'
# Бинарник не исполняется упаковщиком, в том числе при cross-build.
post=(R/'server/packaging/postinst').read_bytes();pre=(R/'server/packaging/prerm').read_bytes();postrm=(R/'server/packaging/postrm').read_bytes();install=(R/'server/packaging/arch.install').read_bytes()

def tar_bytes(entries):
 buf=io.BytesIO()
 with tarfile.open(fileobj=buf,mode='w',format=tarfile.GNU_FORMAT)as t:
  dirs=set()
  for name in entries:
   assert not name.startswith('/')and'..'not in Path(name).parts,name
   parent=Path(name).parent
   while str(parent)!='.':dirs.add(str(parent));parent=parent.parent
  for name in sorted(dirs,key=lambda x:(x.count('/'),x)):
   h=tarfile.TarInfo(name);h.type=tarfile.DIRTYPE;h.mode=0o755;h.uid=h.gid=0;h.uname=h.gname='root';h.mtime=stamp;t.addfile(h)
  for name,(data,mode)in sorted(entries.items()):
   h=tarfile.TarInfo(name);h.size=len(data);h.mode=mode;h.uid=h.gid=0;h.uname=h.gname='root';h.mtime=stamp;t.addfile(h,io.BytesIO(data))
 return buf.getvalue()

def ar(parts):
 result=bytearray(b'!<arch>\n')
 for name,data in parts:
  header=f'{name+"/":<16}{stamp:<12}{0:<6}{0:<6}{"100644":<8}{len(data):<10}`\n'.encode();assert len(header)==60
  result.extend(header);result.extend(data)
  if len(data)%2:result.extend(b'\n')
 return result

doc_src=R/'docs/СЕРВЕР.md' if (R/'docs/СЕРВЕР.md').is_file() else R/'README.md'
notice_src=R/'NOTICE.md' if (R/'NOTICE.md').is_file() else R/'shared/telemost/NOTICE.md'
entries={'usr/bin/splitytm-server':(raw,0o755),'usr/lib/systemd/system/splitytm-server.service':((R/'server/systemd/splitytm-server.service').read_bytes(),0o644),'usr/lib/sysusers.d/splitytm.conf':((R/'server/systemd/splitytm.conf').read_bytes(),0o644),'etc/splitytm/server.json':((R/'examples/server.json').read_bytes(),0o640),'usr/share/doc/splitytm-server/'+doc_src.name:(doc_src.read_bytes(),0o644),'usr/share/licenses/splitytm-server/COPYING':((R/'LICENSE').read_bytes(),0o644),'usr/share/licenses/splitytm-server/NOTICE.md':(notice_src.read_bytes(),0o644)}
for f in(R/'LICENSES').rglob('*'):
 if f.is_file():
  assert not f.is_symlink(),'Симлинк в лицензиях требует отдельной проверки'
  entries['usr/share/licenses/splitytm-server/'+f.relative_to(R/'LICENSES').as_posix()]=(f.read_bytes(),0o644)
size=sum(len(data)for data,mode in entries.values())
control=f'''Package: splitytm-server
Version: {V}-1
Architecture: {debarch}
Maintainer: Участники проекта Nekobox YTM <maintainers@example.invalid>
Section: net
Priority: optional
Installed-Size: {(size+1023)//1024}
Depends: systemd (>= 247)
Description: Сервер SplitYTM с независимыми основным и резервным каналами
 Один creator; управление через systemd. Приглашения и cookie задаются владельцем.
'''
md5=''.join(hashlib.md5(data).hexdigest()+'  '+name+'\n'for name,(data,mode)in sorted(entries.items()))
controls={'control':(control.encode(),0o644),'conffiles':(b'/etc/splitytm/server.json\n',0o644),'postinst':(post,0o755),'prerm':(pre,0o755),'postrm':(postrm,0o755),'md5sums':(md5.encode(),0o644)}
deb=O/f'splitytm-server_{V}-1_{debarch}.deb';deb.write_bytes(ar([('debian-binary',b'2.0\n'),('control.tar.gz',gzip.compress(tar_bytes(controls),mtime=stamp)),('data.tar.gz',gzip.compress(tar_bytes(entries),mtime=stamp))]))
info=f'''pkgname = splitytm-server
pkgbase = splitytm-server
pkgver = {V}-1
pkgdesc = Сервер SplitYTM: один creator, основной и облегчённый резерв
builddate = {stamp}
packager = Участники проекта Nekobox YTM
size = {size}
arch = {pacarch}
license = GPL-3.0-or-later
license = MIT
depend = systemd>=247
backup = etc/splitytm/server.json
'''
pe=entries.copy();pe['.PKGINFO']=(info.encode(),0o644);pe['.INSTALL']=(install,0o644)
pac=O/f'splitytm-server-{V}-1-{pacarch}.pkg.tar.zst'
raw_pac = tar_bytes(pe)
if shutil.which('zstd'):
    with pac.open('wb') as out:
        subprocess.run(['zstd', '-q', '-T2', '-10', '-c'], input=raw_pac, stdout=out, check=True)
else:
    try:
        from compression import zstd as _pyzstd  # Python 3.14+
        pac.write_bytes(_pyzstd.compress(raw_pac, level=10))
    except ImportError:
        import zstandard as _zstd
        pac.write_bytes(_zstd.ZstdCompressor(level=10, threads=2).compress(raw_pac))
standalone=O/f'splitytm-server-{V}-linux-{arch}'
if binary!=standalone:shutil.copy2(binary,standalone)
standalone.chmod(0o755)
# Лицензии и пример поставляются также рядом с отдельными бинарниками.
for f in(R/'LICENSES').rglob('*'):
 if f.is_file():
  target=O/'LICENSES'/f.relative_to(R/'LICENSES');target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(f,target);target.chmod(0o644)
for source,name in [(R/'LICENSE','COPYING'),(notice_src,'NOTICE.md'),(doc_src,doc_src.name),(R/'examples/server.json','server-example.json')]:
 target=O/name
 if source.resolve()!=target.resolve():shutil.copyfile(source,target);target.chmod(0o644)
records=[]
for f in[deb,pac,standalone]:
 with f.open('rb')as stream:digest=hashlib.file_digest(stream,'sha256').hexdigest()
 records.append({'name':f.name,'size':f.stat().st_size,'sha256':digest})
(O/('server-packages-'+arch+'.json')).write_text(json.dumps({'arch':arch,'artifacts':records,'installed':False,'binary_executed':False},ensure_ascii=False,indent=2))
(O/('SHA256SUMS-'+arch)).write_text(''.join(x['sha256']+'  '+x['name']+'\n'for x in records))
print('Собраны серверные Debian/Arch и бинарник:',arch,flush=True)
print('Пакеты не устанавливались, сервер не запускался, cookie и рабочие приглашения не читались.',flush=True)
