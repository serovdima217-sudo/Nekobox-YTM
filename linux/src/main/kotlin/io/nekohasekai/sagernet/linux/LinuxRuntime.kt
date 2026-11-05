package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.*
import io.nekohasekai.sagernet.fmt.socks.SOCKSBean
import io.nekohasekai.sagernet.ktx.mkPort
import moe.matsuri.nb4a.proxy.config.ConfigBean
import moe.matsuri.nb4a.proxy.tmsplit.awaitTmSplitSocks
import moe.matsuri.nb4a.utils.JavaUtil.gson
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.*
import java.net.*
import java.nio.file.*
import java.nio.file.attribute.PosixFilePermissions
import java.util.concurrent.TimeUnit

object LinuxRuntime {
 data class Status(val phase:String="Отключено",val profile:Long=0,val rx:Long=0,val tx:Long=0,val error:String="")
 private val scope=CoroutineScope(SupervisorJob()+Dispatchers.IO)
 val status=MutableStateFlow(Status())
 private val connectionCheck=CurrentConnectionCheck(scope)
 val currentConnectionTest=connectionCheck.state
 val logs=MutableStateFlow("")
 private val lock=Mutex()
 private var connection:Job?=null
 @Volatile private var handle:Running?=null
 private data class Running(val process:Process,val directory:Path,val port:Int,val controller:Int,val secret:String,val tun:Boolean,var processRule:String?,var rx:Long=0,var tx:Long=0,var rxBase:Long=0,var txBase:Long=0)
 private val reconfiguration=Mutex()
 private val jobs=mutableSetOf<Job>()
 private fun append(text:String) {
  val safe=text.replace(Regex("[a-zA-Z][a-zA-Z0-9+.-]*://[^\\s]+"),"[ссылка скрыта]")
  synchronized(logs){logs.value=(logs.value+safe+"\n").takeLast(96000)}
 }
 fun binary(name:String):Path = Path.of(System.getenv("NEKOBOX_YTM_LIBEXEC") ?: "bin",name).toAbsolutePath()
 private fun privateFile(dir:Path,name:String,text:String):Path = dir.resolve(name).also { Files.writeString(it,text);Files.setPosixFilePermissions(it,PosixFilePermissions.fromString("rw-------")) }
 fun physicalInterface():String? = runCatching {
  Files.readAllLines(Path.of("/proc/net/route")).drop(1).map{it.trim().split(Regex("\\s+"))}.filter{it.size>7&&it[1]=="00000000"&&it[7]=="00000000"&&!it[0].startsWith("nbx")}.minByOrNull{it[6].toIntOrNull()?:Int.MAX_VALUE}?.first()
 }.getOrNull()
 /** Коррекция только платформенных параметров. Все outbounds/DNS/rules строит исходный ConfigBuilder Android. */
 fun linuxConfig(entity:ProxyEntity,port:Int,tun:Boolean,controller:Int,secret:String,processSettings:LinuxProcessBypass.Settings=LinuxProcessBypass.settings(),built:ConfigBuildResult?=null):JSONObject {
  val config=JSONObject((built?:buildConfig(entity,forTest=!tun)).config)
  val inbounds=config.optJSONArray("inbounds")?:JSONArray().also{config.put("inbounds",it)}
  if(!tun) { while(inbounds.length()>0)inbounds.remove(0);inbounds.put(JSONObject().put("type","mixed").put("tag","mixed-in").put("listen","127.0.0.1").put("listen_port",port)) }
  else {
   for(i in 0 until inbounds.length()) {
    val item=inbounds.getJSONObject(i)
    if(item.optString("type")=="mixed") item.put("listen_port",port)
    if(item.optString("type")=="tun") {
     val addresses=JSONArray();for(key in listOf("inet4_address","inet6_address")){val values=item.optJSONArray(key);if(values!=null)for(j in 0 until values.length())addresses.put(values[j]);item.remove(key)}
     if(DataStore.ipv6Mode==0 && runCatching{Files.readString(Path.of("/proc/sys/net/ipv6/conf/all/disable_ipv6")).trim()=="0"}.getOrDefault(false))addresses.put("fdfe:dcba:9876::1/126")
     item.put("address",addresses).put("interface_name","nbxytm0").put("auto_route",true).put("auto_redirect",true).put("strict_route",false).put("exclude_uid",JSONArray().put(0))
     item.remove("endpoint_independent_nat")
     if(DataStore.bypassLan)item.put("route_exclude_address",JSONArray(listOf("10.0.0.0/8","172.16.0.0/12","192.168.0.0/16","169.254.0.0/16","fc00::/7","fe80::/10")))
    }
   }
  }
  val route=config.optJSONObject("route")?:JSONObject().also{config.put("route",it)}
  route.put("auto_detect_interface",true).put("find_process",true)
  if(tun&&DataStore.ipv6Mode==0){val rules=route.optJSONArray("rules")?:JSONArray().also{route.put("rules",it)};rules.put(JSONObject().put("ip_version",6).put("action","reject"))}
  physicalInterface()?.let{route.put("default_interface",it)}
  if(controller>0) {
   val experimental=config.optJSONObject("experimental")?:JSONObject().also{config.put("experimental",it)}
   experimental.put("clash_api",JSONObject().put("external_controller","127.0.0.1:$controller").put("secret",secret))
  }
  if(tun)LinuxProcessBypass.apply(config,processSettings)
  return config
 }
 private suspend fun start(entity:ProxyEntity,tun:Boolean,forTest:Boolean=false):Running {
  val dir=Files.createTempDirectory("nekobox-linux-");Files.setPosixFilePermissions(dir,PosixFilePermissions.fromString("rwx------"))
  try {
   LinuxSharing.validateChain(entity)
   val working=entity.copy().putBean(entity.requireBean().clone())
   val selected=working.requireBean()
   val carrier=selected as? ConfigBean
   val port=if(tun||!forTest)DataStore.mixedPort else mkPort()
   val controller=mkPort();val secret=java.util.UUID.randomUUID().toString()
   var carrierFile:Path?=null
   var readinessPort:Int?=null
   val proxy=if(carrier?.type==2) {
    val carrierPort=mkPort();readinessPort=carrierPort
    val json=JSONObject(carrier.config).put("listen","127.0.0.1:$carrierPort")
    physicalInterface()?.let{json.put("bind_interface",it)}
    carrierFile=privateFile(dir,"split.json",json.toString())
    ProxyEntity(id=entity.id,groupId=entity.groupId).putBean(SOCKSBean().apply{initializeDefaultValues();name=carrier.name;serverAddress="127.0.0.1";serverPort=carrierPort})
   } else working
   val built=buildConfig(proxy,forTest=!tun)
   val plugins=LinuxPlugins.prepare(built,dir)
   val redactor=LinuxLogRedactor(listOf(working.requireBean())+built.externalIndex.flatMap{it.chain.values}.map{it.requireBean()})
   val processSettings=LinuxProcessBypass.settings()
   val config=linuxConfig(proxy,port,tun,controller,secret,processSettings,built)
   // В обычном режиме Android-generator добавляет TUN только при serviceMode=vpn.
   val configFile=privateFile(dir,"core.json",config.toString(2))
   val args=mutableListOf<String>()
   if(tun||plugins.needsRoot)args.addAll(listOf("pkexec","--disable-internal-agent"))
   args.addAll(listOf(binary("linux-supervisor").toString(),"--core",binary("nekobox-core").toString(),"--config",configFile.toString(),"--parent",ProcessHandle.current().pid().toString()))
   if(carrierFile!=null)args.addAll(listOf("--carrier",binary("ytm-client").toString(),"--carrier-config",carrierFile.toString()))
   if(plugins.entries.length()>0)args.addAll(listOf("--plugins",privateFile(dir,"plugins.json",plugins.entries.toString()).toString()))
   currentCoroutineContext().ensureActive()
   val process=ProcessBuilder(args).redirectErrorStream(true).start()
   scope.launch { process.inputStream.bufferedReader().useLines{lines->lines.forEach{append(redactor.redact(it))}} }
   val result=Running(process,dir,port,controller,secret,tun,if(tun)processSettings.rule()?.toString()else null)
   try {
    withTimeout(60000) {
     while(process.isAlive) {
      currentCoroutineContext().ensureActive()
      try { awaitTmSplitSocks(listOfNotNull(port,readinessPort)+plugins.ports,1000);return@withTimeout } catch(_:java.io.IOException) { delay(150) }
     }
     error("Сетевой процесс завершился. Подробности в журнале")
    }
    return result
   } catch(t:Throwable){close(result);if(t is TimeoutCancellationException)throw java.io.IOException("Свободная пара SplitYTM не подготовилась за 60 секунд",t);throw t}
  } catch(t:Throwable){dir.toFile().deleteRecursively();throw t}
 }
 private suspend fun close(r:Running) = withContext(NonCancellable+Dispatchers.IO) {
  runCatching{r.process.outputStream.write("Остановить\n".toByteArray());r.process.outputStream.flush();r.process.outputStream.close()}
  if(!r.process.waitFor(12,TimeUnit.SECONDS)) {r.process.destroy();if(!r.process.waitFor(2,TimeUnit.SECONDS))r.process.destroyForcibly()}
  r.directory.toFile().deleteRecursively()
 }
 fun connect(entity:ProxyEntity) {
  connectionCheck.reset()
  if(connection?.isActive==true||handle!=null)return
  connection=scope.launch {
   lock.withLock {
    status.value=Status("Подключение…",entity.id)
    try {
     val running=start(entity,DataStore.serviceMode=="vpn");handle=running
     currentCoroutineContext().ensureActive();DataStore.currentProfile=entity.id
     DataStore.configurationStore.putBoolean("linuxWasConnected",true)
     status.value=Status("Подключено",entity.id)
     while(running.process.isAlive) { delay(1000);updateTraffic(running) }
     error("Соединение остановлено сетевым процессом")
    } catch(_:CancellationException){status.value=Status("Отключено")}
    catch(t:Throwable){append(t.message?:"Ошибка подключения");status.value=Status(error=t.message?:"Ошибка подключения")}
    finally{handle?.let{r->withContext(NonCancellable){runCatching{SagerDatabase.proxyDao.getById(entity.id)?.let{saved->saved.rx+=r.rx;saved.tx+=r.tx;SagerDatabase.proxyDao.updateProxy(saved)}};close(r)}};handle=null;if(status.value.phase!="Отключено")status.value=status.value.copy(phase="Отключено",profile=0)}
   }
  }
 }
 /** Обновляет только маршруты ядра. Клиент и занятая пара SplitYTM остаются прежними. */
 suspend fun applyProcessBypass():Boolean = reconfiguration.withLock { withContext(Dispatchers.IO) {
  withTimeout(65000){while(handle==null&&status.value.phase=="Подключение…")delay(100)}
  val running=handle?:return@withContext false
  if(!running.tun)return@withContext false
  val path=running.directory.resolve("core.json")
  val previous=Files.readString(path)
  val config=JSONObject(previous)
  val nextRule=LinuxProcessBypass.apply(config,LinuxProcessBypass.settings(),running.processRule)
  val candidate=privateFile(running.directory,"core.next.json",config.toString(2))
  val report=privateFile(running.directory,"core-check.log","")
  try {
   val builder=ProcessBuilder(binary("nekobox-core").toString(),"check","-c",candidate.toString()).redirectErrorStream(true).redirectOutput(report.toFile())
   builder.environment().putAll(mapOf("ENABLE_DEPRECATED_LEGACY_DNS_SERVERS" to "true","ENABLE_DEPRECATED_SPECIAL_OUTBOUNDS" to "true"))
   val checker=builder.start()
   if(!checker.waitFor(20,TimeUnit.SECONDS)){checker.destroyForcibly();checker.waitFor();error("Ядро не завершило проверку новых маршрутов")}
   if(checker.exitValue()!=0){append(Files.readString(report));error("Ядро отклонило исключения; прежние маршруты сохранены")}
   check(handle===running&&running.process.isAlive){"Подключение изменилось; повторите применение"}
   updateTraffic(running)
   Files.move(candidate,path,StandardCopyOption.REPLACE_EXISTING,StandardCopyOption.ATOMIC_MOVE)
   try {
    synchronized(running.process){running.process.outputStream.write("Перезагрузить\n".toByteArray());running.process.outputStream.flush()}
   }catch(t:Throwable){privateFile(running.directory,"core.json",previous);throw t}
   running.processRule=nextRule
   running.rxBase=running.rx;running.txBase=running.tx
   delay(700)
   awaitTmSplitSocks(listOf(running.port),12000)
   check(handle===running&&running.process.isAlive){"Сетевой процесс завершился при обновлении маршрутов"}
   updateTraffic(running)
   append("Исключения Linux обновлены; пара SplitYTM не переподключалась")
   true
  }finally{Files.deleteIfExists(candidate);Files.deleteIfExists(report)}
 }}
 fun switchProfile(entity:ProxyEntity){scope.launch{connection?.cancelAndJoin();connect(entity)}}
 fun disconnect() {DataStore.configurationStore.putBoolean("linuxWasConnected",false);connectionCheck.reset();status.value=status.value.copy(phase="Отключение…");connection?.cancel()}
 fun activeProxy():InetSocketAddress?=handle?.takeIf{status.value.phase=="Подключено"}?.let{InetSocketAddress("127.0.0.1",it.port)}
 fun allowDashboardSocket(uri:java.net.URI):Boolean {
  val r=handle?:return false
  return uri.scheme=="ws"&&uri.host=="127.0.0.1"&&uri.port==r.controller&&uri.path in setOf("/traffic","/connections","/memory","/logs")&&NativeWebSockets.query(uri)["token"]==r.secret
 }
 fun dashboardURL(configured:String):String {
  if(configured !in listOf("http://127.0.0.1:9090/ui","http://127.0.0.1:9090/ui/"))return configured
  val base=AndroidDashboard.url();val running=handle
  return if(running!=null)base+"?hostname=127.0.0.1&port=${running.controller}&secret=${running.secret}&lng=ru"else base+"?lng=ru"
 }
 private fun updateTraffic(r:Running) {
  val current=status.value
  val totals=runCatching {
   val connection=java.net.URL("http://127.0.0.1:${r.controller}/connections").openConnection(Proxy.NO_PROXY)as java.net.HttpURLConnection
   try{connection.connectTimeout=400;connection.readTimeout=400;connection.setRequestProperty("Authorization","Bearer ${r.secret}")
    connection.inputStream.bufferedReader().use{JSONObject(it.readText())}
   }finally{connection.disconnect()}
  }.getOrNull()
  if(totals!=null){r.rx=r.rxBase+totals.optLong("downloadTotal");r.tx=r.txBase+totals.optLong("uploadTotal");status.value=current.copy(rx=r.rx,tx=r.tx)}
  else if(Files.exists(Path.of("/sys/class/net/nbxytm0/statistics/rx_bytes"))) {
   fun read(name:String)=runCatching{Files.readString(Path.of("/sys/class/net/nbxytm0/statistics/$name")).trim().toLong()}.getOrDefault(0)
   r.rx=r.rxBase+read("tx_bytes");r.tx=r.txBase+read("rx_bytes");status.value=current.copy(rx=r.rx,tx=r.tx)
  }
 }
 internal suspend fun request(port:Int,url:String):Int = withContext(Dispatchers.IO) {
  val client=OkHttpClient.Builder().proxy(Proxy(Proxy.Type.SOCKS,InetSocketAddress("127.0.0.1",port))).connectTimeout(20,TimeUnit.SECONDS).readTimeout(20,TimeUnit.SECONDS).callTimeout(20,TimeUnit.SECONDS).build()
  val call=client.newCall(Request.Builder().url(url).get().build())
  val start=System.nanoTime()
  try { suspendCancellableCoroutine<Int>{cont->
   cont.invokeOnCancellation{call.cancel()}
   call.enqueue(object:okhttp3.Callback {
    override fun onFailure(call:okhttp3.Call,e:java.io.IOException){if(cont.isActive)cont.resumeWith(Result.failure(e))}
    override fun onResponse(call:okhttp3.Call,response:okhttp3.Response){response.use { if(cont.isActive) {
     if(it.code in 200..399)cont.resumeWith(Result.success(((System.nanoTime()-start)/1_000_000).toInt()))
     else cont.resumeWith(Result.failure(java.io.IOException("HTTP ${it.code}")))
    } } }
   })
  } } finally { client.dispatcher.executorService.shutdownNow();client.connectionPool.evictAll();client.cache?.close() }
 }
 /** Проверка действующего соединения не записывает ping/status/error в профиль. */
 fun testCurrentConnection() {
  val running=handle?:return
  if(status.value.phase!="Подключено")return
  connectionCheck.start(status.value.profile,{request(running.port,DataStore.connectionTestURL)},{handle===running&&status.value.phase=="Подключено"})
 }
 suspend fun urlTest(entity:ProxyEntity):Int {
  val running=handle
  if(running!=null&&status.value.profile==entity.id) return request(running.port,DataStore.connectionTestURL)
  val temporary=start(entity,false,true)
  return try{request(temporary.port,DataStore.connectionTestURL)}finally{close(temporary)}
 }
 internal suspend fun awaitStopped(){connection?.join()}
 suspend fun shutdown(){connectionCheck.close();connection?.cancelAndJoin();synchronized(jobs){jobs.toList()}.forEach{it.cancelAndJoin()};scope.cancel()}
}
