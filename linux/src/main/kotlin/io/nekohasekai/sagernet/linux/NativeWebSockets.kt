package io.nekohasekai.sagernet.linux

import com.sun.net.httpserver.HttpExchange
import java.net.*
import java.net.http.*
import java.util.*
import java.util.concurrent.*
import org.json.JSONObject

/** Linux-эквивалент WebSocket Android WebView: JDK transport, исходный интерфейс yacd. */
object NativeWebSockets {
 private val client=HttpClient.newBuilder().connectTimeout(java.time.Duration.ofSeconds(5)).executor(Executors.newCachedThreadPool{r->Thread(r,"Сокеты панели").apply{isDaemon=true}}).build()
 private val sockets=ConcurrentHashMap<String,WebSocket>()
 private val slots=Semaphore(16)
 fun query(uri:URI):Map<String,String> = uri.rawQuery.orEmpty().split('&').filter{it.contains('=')}.associate{val p=it.split('=',limit=2);URLDecoder.decode(p[0],"UTF-8") to URLDecoder.decode(p[1],"UTF-8")}
 fun handle(e:HttpExchange):Boolean {
  if(!e.requestURI.path.startsWith("/__ws"))return false
  val q=query(e.requestURI)
  if(e.requestURI.path!="/__ws"){
   val socket=sockets[q["id"]]
   if(e.requestMethod!="POST"||socket==null){e.sendResponseHeaders(404,-1);e.close();return true}
   try{
    if(e.requestURI.path=="/__ws/close")socket.sendClose(WebSocket.NORMAL_CLOSURE,"")
    else if(e.requestURI.path=="/__ws/send"){
     val body=e.requestBody.readNBytes(65537);require(body.size<=65536);socket.sendText(body.toString(Charsets.UTF_8),true)
    }
    e.sendResponseHeaders(204,-1)
   }finally{e.close()};return true
  }
  val target=runCatching{URI(q["url"].orEmpty())}.getOrNull()
  if(e.requestMethod!="GET"||target==null||!LinuxRuntime.allowDashboardSocket(target)||!slots.tryAcquire()){
   e.sendResponseHeaders(403,-1);e.close();return true
  }
  val id=UUID.randomUUID().toString();val done=CountDownLatch(1);var socket:WebSocket?=null
  try{
   e.responseHeaders.set("Content-Type","text/event-stream; charset=utf-8");e.responseHeaders.set("Cache-Control","no-store");e.sendResponseHeaders(200,0)
   val output=e.responseBody;val lock=Any()
   fun event(type:String,data:String){synchronized(lock){output.write("event: $type\ndata: $data\n\n".toByteArray());output.flush()}}
   socket=client.newWebSocketBuilder().connectTimeout(java.time.Duration.ofSeconds(5)).buildAsync(target,object:WebSocket.Listener{
    val text=StringBuilder()
    override fun onOpen(ws:WebSocket){sockets[id]=ws;try{event("wsopen",JSONObject().put("id",id).toString());ws.request(1)}catch(_:Exception){done.countDown();ws.abort()}}
    override fun onText(ws:WebSocket,data:CharSequence,last:Boolean):CompletionStage<*>?{
     try{text.append(data);require(text.length<=8*1024*1024);if(last){event("wsmessage",JSONObject.quote(text.toString()));text.setLength(0)};ws.request(1)}catch(_:Exception){done.countDown();ws.abort()};return CompletableFuture.completedFuture(null)
    }
    override fun onClose(ws:WebSocket,statusCode:Int,reason:String):CompletionStage<*>?{runCatching{event("wsclose",JSONObject().put("code",statusCode).put("reason",reason).toString())};done.countDown();return CompletableFuture.completedFuture(null)}
    override fun onError(ws:WebSocket,error:Throwable){runCatching{event("wserror","{}")} ;done.countDown()}
   }).get(7,TimeUnit.SECONDS)
   while(!done.await(5,TimeUnit.SECONDS)){synchronized(lock){output.write(": пульс\n\n".toByteArray());output.flush()}}
  }catch(_:Exception){runCatching{e.responseBody.write("event: wserror\ndata: {}\n\n".toByteArray());e.responseBody.flush()}}
  finally{socket?.abort();sockets.remove(id);e.close();slots.release()}
  return true
 }
 fun close(){sockets.values.forEach{it.abort()};sockets.clear()}
}
