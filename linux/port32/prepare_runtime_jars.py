"""Подготовка зависимостей полного 32-битного GUI из собранных Skiko и OpenJFX SDK.
SPDX-License-Identifier: GPL-3.0-or-later
"""
from pathlib import Path
import argparse,os,shutil,zipfile,json,struct
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--arch',choices=['x86','armv7'],required=True);p.add_argument('--fx-sdk',type=Path,required=True);p.add_argument('--output-dir',type=Path,required=True);a=p.parse_args()
repository=Path(__file__).resolve().parents[2];work=Path(os.environ['NEKOBOX_PORT32_WORK']).resolve();library=a.fx_sdk/'lib/libjfxwebkit.so';assert library.is_file(),'Сначала соберите полный JavaFX SDK с WebKit'
with library.open('rb')as stream:header=stream.read(20)
machine=3 if a.arch=='x86'else 40;assert header[:5]==b'\x7fELF\x01'and struct.unpack_from('<H',header,18)[0]==machine,'SDK содержит чужой WebKit ABI'
for name in ['libjfxmedia.so','libgstreamer-lite.so','libfxplugins.so']:
 media=a.fx_sdk/'lib'/name;assert media.is_file(),'В SDK отсутствует обязательный Media runtime: '+name
 with media.open('rb')as stream:head=stream.read(20)
 assert head[:5]==b'\x7fELF\x01'and struct.unpack_from('<H',head,18)[0]==machine,'Чужой Media ABI: '+name
assert not a.output_dir.exists(),'Выберите новый выходной каталог, существующие файлы автоматически не удаляются'
libs=work/'skiko-9a5b398bb2044fff7e7a84fbfd6f4b803e4427c0/skiko/build/libs';awt=[f for f in libs.glob('skiko-awt-*.jar')if'runtime'not in f.name and not any(x in f.name for x in ['sources','javadoc'])and not f.name.startswith('skiko-awt-test-')];assert len(awt)==1,'Требуется однозначный результат задачи awtJar'
arch='x86'if a.arch=='x86'else'arm32';native=list(libs.glob('skiko-*-linux-'+arch+'.jar'));assert len(native)==1,'Не найден однозначный Skiko JNI JAR целевой архитектуры'
a.output_dir.mkdir(parents=True)
for f in(repository/'linux/runtime-jars').glob('*.jar'):
 if not f.name.startswith(('javafx','skiko')):shutil.copy2(f,a.output_dir/f.name)
for name in ['javafx.base','javafx.graphics','javafx.controls','javafx.media','javafx.web','javafx.swing']:
 f=a.fx_sdk/'lib'/(name+'.jar');assert f.is_file(),'В SDK отсутствует модуль: '+name;shutil.copy2(f,a.output_dir/f.name)
for f in [*awt,*native]:shutil.copy2(f,a.output_dir/f.name)
(a.output_dir/'подготовка.json').write_text(json.dumps({'arch':a.arch,'jars':str(a.output_dir.resolve()),'extra_native_dir':str((a.fx_sdk/'lib').resolve()),'note':'Это зависимости; приложение, ядро, плагины и полная QA проверяются отдельно.'},ensure_ascii=False,indent=2))
print('Подготовлены целевые JAR. Для упаковки передайте SDK/lib через --extra-native-dir.')
