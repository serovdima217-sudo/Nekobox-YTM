package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.*
import io.nekohasekai.sagernet.fmt.socks.SOCKSBean
import io.nekohasekai.sagernet.ktx.*
import moe.matsuri.nb4a.proxy.config.ConfigBean
import moe.matsuri.nb4a.proxy.tmsplit.*
import kotlinx.coroutines.*
import org.json.*
import java.nio.file.*
import java.net.InetSocketAddress
import java.util.concurrent.TimeUnit

/** Автономные проверки используют только специально указанную пустую QA-базу. */
object SelfTest {
 fun run() = runBlocking {
  val state=System.getenv("NEKOBOX_LINUX_STATE")?:error("Для проверки нужна отдельная QA-база")
  require(Path.of(state).fileName.toString().startsWith("qa-")){"Запрещено тестировать пользовательскую базу"}
  require(SagerDatabase.proxyDao.getAll().isEmpty()){ "QA-база должна быть пустой" }
  var count=0
  fun passed(name:String){count++;println("Проверка $count: $name — успешно")}
  val group=ProxyGroup(name="Проверки",ungrouped=true);group.id=SagerDatabase.groupDao.createGroup(group)
  for((name,factory)in protocolChoices()) {
   val bean=factory().applyDefaultValues();bean.name="Проверка $name"
   val bytes=KryoConverters.serialize(bean)
   val clone=bean.clone()
   check(KryoConverters.serialize(clone).contentEquals(bytes)){"Изменился двоичный формат $name"}
   val entity=ProxyEntity(groupId=group.id).putBean(bean)
   entity.id=SagerDatabase.proxyDao.addProxy(entity)
   check(SagerDatabase.proxyDao.getById(entity.id)!!.requireBean().javaClass==bean.javaClass)
   passed("Модель, Kryo и SQLite: $name")
  }
  for((name,factory)in protocolChoices()) {
   val bean=factory().applyDefaultValues();val editor=editorFor(bean)?:continue
   val initial=KryoConverters.serialize(bean);editor.load(bean);editor.save(bean)
   check(KryoConverters.serialize(bean).contentEquals(initial)){"Редактор изменил неизменённый профиль: $name"}
   passed("Исходный init/serialize без потерь: $name")
  }
  DataStore.mixedPort=22080;check(DataStore.mixedPort==22080)
  DataStore.mtu=1480;check(DataStore.mtu==1480)
  passed("Настройки и исходные property-delegates")
  val socks=SOCKSBean().applyDefaultValues().apply{name="Редактор";serverAddress="127.0.0.1";serverPort=1080;username="пользователь";password="пароль";sUoT=true}
  val entity=ProxyEntity(groupId=group.id).putBean(socks)
  val editor=LinuxProfiles.editorLoad(entity)!!
  check(DataStore.serverUsername=="пользователь")
  DataStore.serverUsername="изменено";editor.save(socks);check(socks.username=="изменено")
  passed("Оригинальный init/serialize SOCKS-редактора")
  val source="""{"type":"tm-split","name":"Проверка","links":["https://telemost.yandex.ru/j/100","https://telemost.yandex.ru/j/200"]}"""
  val split=parseTmSplit(source);check(parseTmSplit(split.tmSplitUri()).config==split.config)
  check(runCatching{parseTmSplit(source.replace("/200","/100"))}.isFailure)
  check(runCatching{parseTmSplit(source.replace("/200","/abc"))}.isFailure)
  passed("Оригинальный парсер SplitYTM, URI и проверка пар")
  check(LinuxProfiles.parse("socks5://127.0.0.1:1080#Проверка").size==1)
  check(LinuxProfiles.parse("proxies:\n  - name: Проверка\n    type: socks5\n    server: 127.0.0.1\n    port: 1080\n").size==1)
  passed("Оригинальные парсеры ссылок и Clash YAML")
  check(AndroidPreferences.screen("socks_preferences").any{it.key=="serverPassword"})
  check(AndroidPreferences.screen("global_preferences").isNotEmpty())
  passed("Исходные Android XML-экраны")
  val root=Path.of(state)
  val config=LinuxRuntime.linuxConfig(entity,23080,false,0,"")
  val path=root.resolve("generated.json");Files.writeString(path,config.toString(2))
  val check=ProcessBuilder(LinuxRuntime.binary("nekobox-core").toString(),"check","-c",path.toString()).redirectErrorStream(true)
  check.environment()["ENABLE_DEPRECATED_LEGACY_DNS_SERVERS"]="true"
  check.environment()["ENABLE_DEPRECATED_SPECIAL_OUTBOUNDS"]="true"
  val process=check.start();val output=process.inputStream.bufferedReader().readText()
  check(process.waitFor()==0){"Ядро отвергло исходную конфигурацию: $output"}
  passed("ConfigBuilder Android → Linux → настоящее ядро sing-box check")
  ProcessBypassTest.run();passed("Исключения Linux по исполняемым файлам и безопасное обновление правил")
  val http=com.sun.net.httpserver.HttpServer.create(InetSocketAddress("127.0.0.1",0),0)
  http.createContext("/test"){exchange->exchange.sendResponseHeaders(204,-1);exchange.close()};http.start()
  val upstreamPort=mkPort()
  val upstream=root.resolve("upstream.json");Files.writeString(upstream,JSONObject().put("inbounds",JSONArray().put(JSONObject().put("type","mixed").put("listen","127.0.0.1").put("listen_port",upstreamPort))).put("outbounds",JSONArray().put(JSONObject().put("type","direct"))).toString())
  val upstreamProcess=ProcessBuilder(LinuxRuntime.binary("nekobox-core").toString(),"run","-c",upstream.toString()).redirectErrorStream(true).redirectOutput(root.resolve("upstream.log").toFile()).start()
  try {
   awaitTmSplitSocks(listOf(upstreamPort),10000)
   socks.serverPort=upstreamPort
   DataStore.connectionTestURL="http://127.0.0.1:${http.address.port}/test"
   val latency=LinuxRuntime.urlTest(entity)
   check(latency>=0);passed("URL-тест через два настоящих SOCKS-ядра: $latency мс")
  }finally{upstreamProcess.destroy();upstreamProcess.waitFor(5,TimeUnit.SECONDS);if(upstreamProcess.isAlive)upstreamProcess.destroyForcibly();http.stop(0);LinuxRuntime.shutdown()}
  println("Все выполненные проверки: $count. Боевые настройки не затронуты.")
 }
}
