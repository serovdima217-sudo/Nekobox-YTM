"""Изолированная проверка TCP/UDP IPv4/IPv6, не меняющая работающий VPN хоста."""
from pathlib import Path
import json, os, pwd, shutil, signal, socket, socketserver, struct, subprocess, sys, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

assert os.geteuid() == 0, 'Проверка запускается через sudo'
qa_user = os.environ.get('SUDO_USER')
assert qa_user and qa_user != 'root', 'Запустите через sudo от обычного пользователя'
qa_identity = pwd.getpwnam(qa_user)
root = Path(__file__).resolve().parent
state = Path(sys.argv[1]).resolve()
assert state.name.startswith('qa-') and state.parent == root
expected = json.loads((state/'process-bypass-expected.json').read_text())
rule = json.loads((state/'process-bypass-rule.json').read_text())
selected_value='VPN' if '--no-inherit' in sys.argv else 'DIRECT'
server_ns='nbx-proc-server-'+str(os.getpid()); original_ns=os.open('/proc/self/ns/net',os.O_RDONLY); server_created=False
ns = 'nbx-proc-'+str(os.getpid()); host = 'np'+str(os.getpid())+'h'; peer='np'+str(os.getpid())+'n'
gateway='10.253.247.1'; local='10.253.247.2'; target='198.51.100.77'; target6='2001:db8:7ffe:77::1'
base_rules=subprocess.check_output(['ip','rule'], text=True)
baseline_tun=subprocess.check_output(['ip','-br','link','show','type','tun'],text=True)
services=[]; started_services=[]; child=None; created=False
signal.signal(signal.SIGTERM,lambda *_:sys.exit(143))

def run(args, **kw):
    return subprocess.run(args,check=True,capture_output=True,text=True,timeout=20,**kw)
def exact(s,n):
    result=b''
    while len(result)<n:
        data=s.recv(n-len(result))
        if not data:raise EOFError()
        result+=data
    return result

def address(s,atyp):
    if atyp==1: raw=exact(s,4); name=socket.inet_ntop(socket.AF_INET,raw); encoded=b'\x01'+raw
    elif atyp==4:raw=exact(s,16); name=socket.inet_ntop(socket.AF_INET6,raw); encoded=b'\x04'+raw
    elif atyp==3:
        size=exact(s,1);raw=exact(s,size[0]);name=raw.decode();encoded=b'\x03'+size+raw
    else:raise ValueError('Неверный адрес')
    port=exact(s,2)
    return name,struct.unpack('!H',port)[0],encoded+port

class Http(BaseHTTPRequestHandler):
    def do_GET(self):
        body=b'DIRECT';self.send_response(200);self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
    def log_message(self,*args):pass
class FastHTTP(ThreadingHTTPServer):
    def server_bind(self):
        socketserver.TCPServer.server_bind(self)
        self.server_name='QA';self.server_port=self.server_address[1]
class Http6(FastHTTP):address_family=socket.AF_INET6
class Socks(socketserver.BaseRequestHandler):
    def handle(self):
        s=self.request;s.settimeout(15)
        try:
            ver,n=exact(s,2);exact(s,n);s.sendall(b'\x05\x00')
            ver,command,_,kind=exact(s,4);name,port,encoded=address(s,kind)
            if command==1:
                s.sendall(b'\x05\x00\x00\x01'+b'\x00'*6)
                data=b''
                while b'\r\n\r\n' not in data:
                    data+=s.recv(4096)
                    if not data: return
                s.sendall(b'HTTP/1.1 200 OK\r\nContent-Length: 3\r\nConnection: close\r\n\r\nVPN')
            elif command==3:
                udp=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);udp.bind((gateway,0));udp.settimeout(.3)
                done=threading.Event()
                def relay():
                    while not done.is_set():
                        try:
                            data,client=udp.recvfrom(65535)
                            if data[3]==1:end=10
                            elif data[3]==4:end=22
                            elif data[3]==3:end=7+data[4]
                            else:continue
                            udp.sendto(data[:end]+b'VPN',client)
                        except socket.timeout:continue
                        except OSError:return
                thread=threading.Thread(target=relay,daemon=True);thread.start()
                s.sendall(b'\x05\x00\x00\x01'+socket.inet_aton(gateway)+struct.pack('!H',udp.getsockname()[1]))
                try:
                    while s.recv(1024):pass
                finally:done.set();udp.close();thread.join(1)
        except (OSError,EOFError,ValueError):pass
class SocksServer(socketserver.ThreadingTCPServer):
    allow_reuse_address=True
    daemon_threads=True

try:
    run(['ip','netns','add',ns]);created=True
    run(['ip','netns','add',server_ns]);server_created=True
    run(['ip','link','add',host,'type','veth','peer','name',peer])
    run(['ip','link','set',peer,'netns',ns]);run(['ip','link','set',host,'netns',server_ns])
    run(['ip','-n',server_ns,'addr','add',gateway+'/30','dev',host]);run(['ip','-n',server_ns,'addr','add',target+'/32','dev',host])
    run(['ip','-n',server_ns,'-6','addr','add','2001:db8:7fff:77::1/64','dev',host,'nodad']);run(['ip','-n',server_ns,'-6','addr','add',target6+'/128','dev',host,'nodad'])
    run(['ip','-n',server_ns,'link','set',host,'up']);run(['ip','-n',server_ns,'link','set','lo','up'])
    for args in [['link','set','lo','up'],['addr','add',local+'/30','dev',peer],['-6','addr','add','2001:db8:7fff:77::2/64','dev',peer,'nodad'],['link','set',peer,'up'],['route','add','default','via',gateway],['-6','route','add','default','via','2001:db8:7fff:77::1']]:run(['ip','-n',ns]+args)
    with open('/var/run/netns/'+server_ns,'rb')as descriptor:os.setns(descriptor.fileno(),0)
    socks=SocksServer((gateway,0),Socks);services.append(socks)
    http=FastHTTP((target,0),Http);services.append(http)
    http6=Http6((target6,0),Http);services.append(http6)
    for service in services:
        threading.Thread(target=service.serve_forever,daemon=True).start();started_services.append(service)
    udp_ports={};udp_sockets=[]
    for family,ip in [(socket.AF_INET,target),(socket.AF_INET6,target6)]:
        udp=socket.socket(family,socket.SOCK_DGRAM);udp.bind((ip,0));udp_ports[family]=udp.getsockname()[1];udp_sockets.append(udp)
        def direct(sock):
            try:
                while True:
                    data,client=sock.recvfrom(65535);sock.sendto(b'DIRECT',client)
            except OSError:pass
        threading.Thread(target=direct,args=(udp,),daemon=True).start()
    for key,source in [('selected_curl',shutil.which('curl')),('selected_python',str(Path(sys.executable).resolve()))]:
        dest=Path(expected[key]);dest.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(source,dest);dest.chmod(0o755);os.chown(dest.parent,qa_identity.pw_uid,qa_identity.pw_gid)
    config={'log':{'level':'info','timestamp':False},'inbounds':[{'type':'tun','tag':'tun-in','interface_name':'nbxqa0','address':['172.29.243.1/30','fdfe:abcd:243::1/126'],'auto_route':True,'auto_redirect':True,'strict_route':False,'exclude_uid':[0]}],
            'outbounds':[{'type':'socks','tag':'proxy','server':gateway,'server_port':socks.server_address[1]},{'type':'direct','tag':'direct'}],
            'route':{'find_process':True,'default_interface':peer,'rules':[rule],'final':'proxy'}}
    path=state/'namespace-core.json';path.write_text(json.dumps(config));log=state/'namespace.log';output=log.open('w')
    child=subprocess.Popen(['ip','netns','exec',ns,str(root/'bin/linux-supervisor'),'--core',str(root/'bin/nekobox-core'),'--config',str(path),'--parent',str(os.getpid())],stdin=subprocess.PIPE,stdout=output,stderr=subprocess.STDOUT,text=True)
    def request(binary,ip,port,udp=False):
        command=['ip','netns','exec',ns,'sudo','-u',qa_user]
        if binary in [expected['selected_curl'],expected['selected_python']]:
            launcher=state/'parent-launcher';shutil.copy2(root/'.qa-tools/parent-launcher-template',launcher);launcher.chmod(0o755);os.chown(state,qa_identity.pw_uid,qa_identity.pw_gid)
            command+=[str(launcher)]+(['--unlink-self']if '--deleted-parent' in sys.argv else[])
        command+=[binary]
        if udp:
            family='socket.AF_INET6'if ':'in ip else 'socket.AF_INET'
            code=f"import socket;s=socket.socket({family},socket.SOCK_DGRAM);s.settimeout(4);s.sendto(b'qa',({ip!r},{port}));print(s.recvfrom(1024)[0].decode())"
            if '--deleted-executable' in sys.argv and binary == expected['selected_python']:
                shutil.copy2(sys.executable, binary)
                code = "import os,sys;os.unlink(sys.executable);" + code
            command+=['-c',code]
        else:
            if '--deleted-executable' in sys.argv and binary == expected['selected_curl']:
                shutil.copy2(shutil.which('curl'),binary)
                command=['ip','netns','exec',ns,'sudo','-u',qa_user,'bash','-c','exec 3<"$1"; rm -- "$1"; shift; exec /proc/self/fd/3 "$@"','qa',binary]
            command+=['--noproxy','*','-fsS','--connect-timeout','3','--max-time','5',f'http://[{ip}]:{port}/'if ':'in ip else f'http://{ip}:{port}/']
        result=subprocess.run(command,capture_output=True,text=True,timeout=9)
        if result.returncode:raise RuntimeError(result.stderr[-350:])
        return result.stdout.strip()
    def wait_for(binary,ip,port,expected_value,udp=False):
        deadline=time.monotonic()+15;last=''
        while time.monotonic()<deadline:
            try:
                last=request(binary,ip,port,udp)
                if last==expected_value:return
            except (RuntimeError,subprocess.TimeoutExpired)as exc:last=str(exc)
            if child.poll()is not None:raise RuntimeError(log.read_text()[-3000:])
            time.sleep(.3)
        raise AssertionError(f'{binary}: ожидалось {expected_value}, получено {last}')
    def check(label,binary,ip,port,value,udp=False):
        wait_for(binary,ip,port,value,udp);print(label+': '+value,flush=True)
    for label,ip,http_port,family in [('IPv4',target,http.server_port,socket.AF_INET),('IPv6',target6,http6.server_port,socket.AF_INET6)]:
        check(label+' TCP выбранный файл',expected['selected_curl'],ip,http_port,selected_value)
        check(label+' TCP обычный curl',shutil.which('curl'),ip,http_port,'VPN')
        check(label+' UDP выбранный файл',expected['selected_python'],ip,udp_ports[family],selected_value,True)
        check(label+' UDP обычный Python',sys.executable,ip,udp_ports[family],'VPN',True)
    for repeat in range(4):
        for ip,http_port,family in [(target,http.server_port,socket.AF_INET),(target6,http6.server_port,socket.AF_INET6)]:
            for binary,value,udp,port in [(expected['selected_curl'],selected_value,False,http_port),(shutil.which('curl'),'VPN',False,http_port),(expected['selected_python'],selected_value,True,udp_ports[family]),(sys.executable,'VPN',True,udp_ports[family])]:
                actual=request(binary,ip,port,udp)
                assert actual==value,(repeat,binary,ip,actual,value)
    print('Повторные 32 проверки без маскирующих повторных попыток пройдены',flush=True)
    before=subprocess.check_output(['pgrep','-P',str(child.pid)],text=True).strip()
    for rules,value in [([], 'VPN'),([rule], selected_value)]:
        config['route']['rules']=rules;temporary=path.with_suffix('.new');temporary.write_text(json.dumps(config));os.replace(temporary,path)
        child.stdin.write('Перезагрузить\n');child.stdin.flush()
        check('Применение без смены процесса ядра',expected['selected_curl'],target,http.server_port,value)
        assert subprocess.check_output(['pgrep','-P',str(child.pid)],text=True).strip()==before
    print('Проверено: только путь лаунчера в правиле, другой дочерний executable, посторонние процессы через VPN, TCP/UDP IPv4/IPv6, совместимость, удалённый лаунчер, SIGHUP без смены PID',flush=True)
finally:
    if child is not None:
        try:child.stdin.write('Остановить\n');child.stdin.flush();child.wait(timeout=15)
        except (OSError,subprocess.TimeoutExpired):child.terminate();child.wait(timeout=10)
    for service in started_services:service.shutdown()
    for service in services:service.server_close()
    for sock in locals().get('udp_sockets',[]):sock.close()
    os.setns(original_ns,0);os.close(original_ns)
    if server_created:subprocess.run(['ip','-n',server_ns,'link','del',host],capture_output=True)
    if created:subprocess.run(['ip','netns','del',ns],capture_output=True)
    if server_created:subprocess.run(['ip','netns','del',server_ns],capture_output=True)
    assert subprocess.check_output(['ip','rule'],text=True)==base_rules,'Изменились правила хостового VPN'
    assert subprocess.check_output(['ip','-br','link','show','type','tun'],text=True)==baseline_tun,'Изменился хостовый TUN'
    print('Хостовый VPN и маршруты не изменены, тестовое пространство удалено',flush=True)
