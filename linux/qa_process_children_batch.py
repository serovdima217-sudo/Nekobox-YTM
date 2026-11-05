"""Реальные пакеты дочерних процессов, три режима, только в отдельных network namespaces."""
from pathlib import Path
import os,subprocess,select,json,time
B=Path(__file__).resolve().parent
(B/'.qa-tools').mkdir(exist_ok=True)
env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOMAXPROCS='2')
subprocess.run(['go','build','-trimpath','-o',str(B/'.qa-tools/parent-launcher-template'),str(B/'tests/parent-launcher.go')],env=env,check=True)
records=[]
for name,inherit,deleted in [('без-флага',False,False),('с-наследованием',True,False),('удалённый-лаунчер',True,True)]:
 q=B/('qa-parent-'+str(time.time_ns()));q.mkdir(mode=0o755);paths=[str(q/'selected/curl'),str(q/'selected/python3')]
 (q/'process-bypass-expected.json').write_text(json.dumps({'selected_curl':paths[0],'selected_python':paths[1]}));rule={'process_path':[str(q/'parent-launcher')],'action':'route','outbound':'direct'}
 if inherit:rule['process_path_include_children']=True
 (q/'process-bypass-rule.json').write_text(json.dumps(rule));args=([] if os.geteuid()==0 else ['sudo','-n'])+['python3','-u',str(B/'qa_process_children.py'),str(q)]+(['--no-inherit']if not inherit else[])+(['--deleted-parent']if deleted else[])
 print('Сетевая проверка:',name,flush=True)
 with(q/'result.log').open('w')as log:p=subprocess.run(args,stdout=log,stderr=subprocess.STDOUT,timeout=240)
 records.append({'mode':name,'code':p.returncode,'state':str(q)});(B/'routing-parent-results.json').write_text(json.dumps(records,ensure_ascii=False,indent=2));print('Результат:',name,p.returncode,flush=True)
 assert p.returncode==0,'Реальная проверка наследования не пройдена'
print('Без флага ребёнок идёт в VPN; с флагом только выбранная семья идёт DIRECT, IPv4/IPv6 TCP/UDP. Хостовый VPN не менялся.',flush=True)
