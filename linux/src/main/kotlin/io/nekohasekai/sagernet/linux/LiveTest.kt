package io.nekohasekai.sagernet.linux
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.*
import moe.matsuri.nb4a.proxy.tmsplit.parseTmSplit
import kotlinx.coroutines.*
import java.nio.file.*
import java.util.concurrent.TimeUnit

/** Использует только переданный закрытый конфиг; на боевую базу не ссылается. */
object LiveTest {
 fun run(path:String,tun:Boolean)=runBlocking {
  val state=System.getenv("NEKOBOX_LINUX_STATE")?:error("Нужна QA-база")
  require(Path.of(state).fileName.toString().startsWith("qa-"))
  DataStore.initGlobal();DataStore.serviceMode=if(tun)"vpn"else"proxy";DataStore.mixedPort=22880;DataStore.mtu=1480;DataStore.bypassLan=true
  DataStore.connectionTestURL="https://example.com/"
  val group=DataStore.currentGroupId()
  val entity=ProxyEntity(groupId=group).putBean(parseTmSplit(Files.readString(Path.of(path))));entity.id=SagerDatabase.proxyDao.addProxy(entity)
  suspend fun waitPhase(phase:String,timeout:Long)=withTimeout(timeout){while(LinuxRuntime.status.value.phase!=phase){val status=LinuxRuntime.status.value;if(status.error.isNotBlank())error(status.error);delay(100)}}
  try {
   if(!tun) {
    LinuxRuntime.connect(entity);delay(400)
    val start=System.nanoTime();LinuxRuntime.disconnect();waitPhase("Отключено",20000)
    println("Отмена реальной подготовки: ${(System.nanoTime()-start)/1_000_000} мс")
   }
   val start=System.nanoTime();LinuxRuntime.connect(entity);waitPhase("Подключено",80000)
   println("Подключение настоящей пары: ${(System.nanoTime()-start)/1_000_000} мс")
   repeat(4){i->val latency=LinuxRuntime.urlTest(entity);println("HTTPS через SplitYTM, запрос ${i+1}: $latency мс");delay(3000)}
   if(tun) {
    fun curlAddress(args:List<String>):String {
     val process=ProcessBuilder(args).redirectErrorStream(true).start();val output=process.inputStream.bufferedReader().readText().trim();check(process.waitFor()==0){"Не удалось проверить внешний адрес: $output"};return output
    }
    val expected=curlAddress(listOf("curl","--noproxy","","--proxy","socks5h://127.0.0.1:${DataStore.mixedPort}","--max-time","20","-fsS","https://api.ipify.org"))
    val actual=curlAddress(listOf("sudo","-u","femboychik","curl","--noproxy","*","-4","--max-time","20","-fsS","https://api.ipify.org"))
    check(expected==actual){"Системный трафик обходит VPN: SOCKS=$expected, система=$actual"}
    println("Независимая проверка маршрутизации: SOCKS=$expected, обычный системный запрос=$actual")
    println("Настоящие счётчики ядра: RX=${LinuxRuntime.status.value.rx}, TX=${LinuxRuntime.status.value.tx}")
    val gateway=ProcessBuilder("ip","route","show","default").start().inputStream.bufferedReader().readText().split(Regex("\\s+")).let{words->words.indexOf("via").takeIf{it>=0}?.let{words.getOrNull(it+1)}}
    fun tunBytes():Long=listOf("rx_bytes","tx_bytes").sumOf { Files.readString(Path.of("/sys/class/net/nbxytm0/statistics/$it")).trim().toLong() }
    repeat(20){cycle->
     val before=tunBytes()
     val curl=ProcessBuilder("sudo","-u","femboychik","curl","--noproxy","*","--connect-timeout","8","--max-time","25","-fsS","-o","/dev/null","-w","%{http_code}","https://example.com/").redirectErrorStream(true).start()
     val output=curl.inputStream.bufferedReader().readText();check(curl.waitFor()==0&&output.trim()=="200"){"Системный TUN-запрос не прошёл, цикл ${cycle+1}: $output"}
     val delta=tunBytes()-before;check(delta>256){"Запрос не прошёл через настоящий TUN: $delta байтов"}
     println("Системный TUN, цикл ${cycle+1}/20: HTTP 200, счётчик интерфейса +$delta байтов (TCP auto-redirect проверен по внешнему IP)")
     if(gateway!=null&&cycle%5==0) {
      val ping=ProcessBuilder("sudo","-u","femboychik","ping","-c","1","-W","2",gateway).redirectErrorStream(true).start()
      val detail=ping.inputStream.bufferedReader().readText();check(ping.waitFor()==0){"Потеряна локальная сеть: $detail"};println("Локальный шлюз доступен без отключения VPN")
     }
     delay(5000)
    }
   }
   LinuxRuntime.disconnect();waitPhase("Отключено",20000)
   println("Отключение и ожидание освобождения служб завершены")
  }catch(t:Throwable){println(LinuxRuntime.logs.value);throw t}finally{LinuxRuntime.shutdown()}
  if(!tun) {
   // Отдельный процесс теста необходим: shutdown окончательно закрывает область coroutine.
   println("Повторное подключение и текущий URL-тест пройдены; боевые настройки не менялись")
  }
 }
}
