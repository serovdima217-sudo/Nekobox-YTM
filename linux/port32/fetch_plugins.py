"""Закреплённые нативные плагины x86/ARMv7, без установки и запуска VPN.
SPDX-License-Identifier: GPL-3.0-or-later
"""
from pathlib import Path
import argparse,os,json,tarfile,zipfile,shutil,subprocess,struct,hashlib
from prepare_inputs import download
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--arch',choices=['x86','armv7','all'],default='all');a=p.parse_args();here=Path(__file__).resolve().parent;W=Path(os.environ['NEKOBOX_PORT32_WORK']).resolve()/'plugins32';W.mkdir(parents=True,exist_ok=True);records=[]
for pin in json.loads((here/'plugins-sources.json').read_text()):
 arch=pin['arch']
 if a.arch not in ['all',arch]:continue
 D=W/arch;D.mkdir(exist_ok=True);archive=download(pin['url'],W/pin['name'],pin['sha256']);assert archive.stat().st_size==pin['size'];component=pin['component'];dest=D/(('naive'if component=='naiveproxy'else component)+'-plugin')
 if pin['build_from_source']:
  source=W/'mieru-3.39.0'
  if not source.exists():
   with tarfile.open(archive)as tar:tar.extractall(W,filter='data')
  env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='386',GO386='softfloat',GOMAXPROCS='2',GOMEMLIMIT='800MiB')
  with(W/'mieru-x86-build.log').open('wb')as out:subprocess.run(['go','build','-trimpath','-ldflags=-s -w','-o',str(dest),'./cmd/mieru'],cwd=source,env=env,stdout=out,stderr=subprocess.STDOUT,check=True)
 else:
  extract=W/(arch+'-'+component);extract.mkdir(exist_ok=True)
  if archive.name.endswith('.zip'):
   with zipfile.ZipFile(archive)as z:
    assert all(not Path(n).is_absolute()and'..'not in Path(n).parts for n in z.namelist());z.extractall(extract)
  elif'.tar.'in archive.name:
   with tarfile.open(archive)as tar:tar.extractall(extract,filter='data')
  else:shutil.copy2(archive,extract/component)
  name='naive'if component=='naiveproxy'else component;binary=next(f for f in extract.rglob(name)if f.is_file());shutil.copy2(binary,dest)
 with dest.open('rb')as stream:header=stream.read(20)
 assert header[:5]==b'\x7fELF\x01'and struct.unpack_from('<H',header,18)[0]==(3 if arch=='x86'else 40),'Плагин имеет чужой ELF ABI';dest.chmod(0o755)
 with dest.open('rb')as stream:digest=hashlib.file_digest(stream,'sha256').hexdigest()
 records.append({'arch':arch,'component':component,'binary_sha256':digest,'input_sha256':pin['sha256']});print('Подготовлен плагин:',arch,component,flush=True)
(W/('результаты-'+a.arch+'.json')).write_text(json.dumps(records,ensure_ascii=False,indent=2));print('Плагины подготовлены. Сетевые проверки выполняются отдельно.')
