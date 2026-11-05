// Платформенный мост WebSocket для WebKit Linux; оформление и код yacd не изменяются.
(function () {
 class LinuxWebSocket extends EventTarget {
  static CONNECTING=0; static OPEN=1; static CLOSING=2; static CLOSED=3;
  constructor(url, protocols) {
   super(); this.url=String(url); this.readyState=0; this.bufferedAmount=0;
   this.binaryType='blob'; this.protocol=''; this.extensions='';
   this.CONNECTING=0; this.OPEN=1; this.CLOSING=2; this.CLOSED=3;
   this.onopen=null; this.onmessage=null; this.onclose=null; this.onerror=null;
   this._events=new EventSource('/__ws?url='+encodeURIComponent(this.url));
   this._events.addEventListener('wsopen', e=>{
    if(this.readyState!==0)return;this._id=JSON.parse(e.data).id;this.readyState=1;this._fire('open',new Event('open'));
   });
   this._events.addEventListener('wsmessage',e=>{
    if(this.readyState===1)this._fire('message',new MessageEvent('message',{data:JSON.parse(e.data),origin:new URL(this.url).origin}));
   });
   this._events.addEventListener('wsclose',e=>{const d=JSON.parse(e.data);this._finish(d.code,d.reason,true)});
   this._events.addEventListener('wserror',()=>this._error());
   this._events.onerror=()=>this._error();
  }
  _fire(name,event){this.dispatchEvent(event);if(typeof this['on'+name]==='function')this['on'+name](event)}
  _error(){if(this.readyState===3)return;this._fire('error',new Event('error'));this._finish(1006,'',false)}
  _finish(code,reason,clean){if(this.readyState===3)return;this.readyState=3;this._events.close();this._fire('close',new CloseEvent('close',{code:code,reason:reason||'',wasClean:clean}))}
  send(data){if(this.readyState!==1)throw new DOMException('Соединение не открыто','InvalidStateError');fetch('/__ws/send?id='+encodeURIComponent(this._id),{method:'POST',body:String(data)}).catch(()=>this._error())}
  close(code=1000,reason=''){if(this._id)fetch('/__ws/close?id='+encodeURIComponent(this._id),{method:'POST'}).catch(()=>{});this._finish(code,reason,true)}
 }
 window.WebSocket=LinuxWebSocket;
})();
