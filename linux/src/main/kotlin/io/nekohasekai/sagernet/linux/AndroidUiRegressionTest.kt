@file:OptIn(androidx.compose.ui.ExperimentalComposeUiApi::class)
package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.foundation.layout.*
import androidx.compose.material.*
import androidx.compose.ui.ImageComposeScene
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.socks.SOCKSBean
import io.nekohasekai.sagernet.ktx.applyDefaultValues
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.first
import org.jetbrains.skia.EncodedImageFormat
import java.net.*
import java.nio.file.*

/** Без окон, трея и TUN: отдельная база, локальный SOCKS-стенд и программная отрисовка. */
object AndroidUiRegressionTest {
 fun run() = runBlocking {
  val root=Path.of(System.getenv("NEKOBOX_LINUX_STATE")?:error("Нужна QA-база"))
  require(root.fileName.toString().startsWith("qa-"));Files.createDirectories(root)
  require(SagerDatabase.proxyDao.getAll().isEmpty())
  DataStore.initGlobal()
  val group=DataStore.currentGroupId()
  val entity=ProxyEntity(groupId=group).putBean(SOCKSBean().applyDefaultValues().apply{name="Контроль TCP/URL";serverAddress="127.0.0.1";serverPort=1080})
  entity.status=1;entity.ping=73;entity.error=null;entity.id=SagerDatabase.proxyDao.addProxy(entity)
  val snapshot=SagerDatabase.proxyDao.getById(entity.id)!!.let{Triple(it.status,it.ping,it.error)}
  fun unchanged(){check(SagerDatabase.proxyDao.getById(entity.id)!!.let{Triple(it.status,it.ping,it.error)}==snapshot){"Нижняя проверка изменила карточку"}}
  for((raw,want)in listOf("SOCKS server general failure" to "Недоступен","Connection refused" to "В соединении отказано","connect timed out" to "Тайм-аут","context deadline exceeded" to "Тайм-аут","UnknownHostException" to "Домен не найден","Network is unreachable" to "Недоступен","HTTP 503" to "Недоступен","В соединении отказано" to "В соединении отказано","Тайм-аут" to "Тайм-аут","Домен не найден" to "Домен не найден"))check(AndroidTestPresentation.profileError(raw)==want){raw}
  val probeScope=CoroutineScope(SupervisorJob()+Dispatchers.IO)
  val probe=CurrentConnectionCheck(probeScope)
  // Бюджет только локального QA-стенда: QEMU медленно инициализирует TLS/JCE; рабочие тайм-ауты клиента не меняются.
  val fixtureTimeout=Integer.getInteger("nekobox.qa.fixtureTimeoutMillis",6000).coerceIn(6000,60000)
  suspend fun finished()=withTimeout(fixtureTimeout.toLong()){probe.state.first{!it.running}}
  try {
   val gate=CompletableDeferred<Int>();var calls=0
   probe.start(entity.id,{calls++;gate.await()},{true});check(probe.state.value.running)
   probe.start(entity.id,{error("Повторная проверка запустилась параллельно")},{true})
   gate.complete(143);check(finished().latency==143);check(calls==1);unchanged()
   // Быстрые последовательные проверки ловят гонку между StateFlow и завершением Job.
   repeat(100){n->probe.start(entity.id,{1000+n},{true});check(finished().latency==1000+n);unchanged()}
   probe.start(entity.id,{throw java.net.SocketTimeoutException("connect timed out")},{true})
   check(AndroidTestPresentation.bottomText("Подключено",finished(),"https://example.invalid")=="Ошибка: Тайм-аут");unchanged()
   val old=CompletableDeferred<Int>()
   probe.start(entity.id,{old.await()},{true});probe.reset();old.complete(999);delay(50)
   check(probe.state.value==CurrentConnectionTest());unchanged()
   probe.start(entity.id,{42},{false});delay(50);check(probe.state.value.latency==null);probe.reset()
   for(failure in listOf(false,true)){
    val server=ServerSocket(0,1,InetAddress.getByName("127.0.0.1")).apply{soTimeout=fixtureTimeout}
    val fixture=probeScope.async {
     server.use{it.accept().use{socket->socket.soTimeout=fixtureTimeout
      val input=socket.getInputStream();val output=socket.getOutputStream()
      check(input.read()==5);val methods=input.read();check(methods>0);input.readNBytes(methods)
      output.write(byteArrayOf(5,0));output.flush()
      val header=input.readNBytes(4);check(header.size==4&&header[0].toInt()==5)
      when(header[3].toInt()){1->input.readNBytes(4);3->input.readNBytes(input.read());4->input.readNBytes(16);else->error("Неверный SOCKS-запрос")};input.readNBytes(2)
      output.write(byteArrayOf(5,if(failure)1 else 0,0,1,127,0,0,1,0,0));output.flush()
      if(!failure){val reader=input.bufferedReader();while(!reader.readLine().isNullOrEmpty()){};output.write("HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n".toByteArray());output.flush()}
     }}
    }
    probe.start(entity.id,{LinuxRuntime.request(server.localPort,"http://connection-test.invalid/generate_204")},{true})
    val result=finished();fixture.await()
    if(failure)check(AndroidTestPresentation.profileError(result.error)=="Недоступен"&&result.latency==null)
    else check(result.latency!=null&&AndroidTestPresentation.bottomText("Подключено",result,"http://connection-test.invalid").startsWith("HTTP-соединение установлено"))
    unchanged()
   }
  }finally{probe.close();probeScope.cancel()}
  val active=LinuxProcessBypass.active()
  check(active.any{ProcessHandle.current().pid()in it.pids});check(active.any{it.uid==0L})
  check(active.map{it.pids.single()}.distinct().size==active.size)
  val hidden=LinuxActiveProcess("@proc/9","Чужой процесс",listOf(9),1001)
  val rootProcess=LinuxActiveProcess("/usr/bin/systemd","systemd",listOf(1),0)
  val twins=listOf(LinuxActiveProcess("/usr/bin/app","app",listOf(100),1000),LinuxActiveProcess("/usr/bin/app","app",listOf(101),1000))
  check(LinuxProcessBypass.entries(twins+hidden+rootProcess,emptySet(),"").size==4)
  check(!hidden.selectable&&!rootProcess.selectable&&twins.all{it.selectable})
  check(LinuxProcessBypass.parseProcesses("[{\"pid\":9,\"uid\":1001,\"path\":\"/usr/bin/app\",\"name\":\"app\"}]").single().selectable)
  check(LinuxProcessBypass.normalize(listOf("@proc/9")).isEmpty())
  for(width in listOf(320,360,458))for(dark in listOf(false,true)){
   var layout:TextLayoutResult?=null
   val scene=ImageComposeScene(width,260,Density(1f)){
    MaterialTheme(colors=if(dark)darkColors()else lightColors(),typography=androidTypography()){
     Surface(color=if(dark)Color(0xFF202020)else Color(0xFFF2F2F2)){
      Column(Modifier.fillMaxSize(),verticalArrangement=Arrangement.Bottom){
       AndroidConnectionBar("Подключено",112233,445566,CurrentConnectionTest(),"https://example.invalid",Color(0xFFDE8670),Color(0xFFD65B37),Color(0xFFFFCCBC),{}, {},onStatusLayout={layout=it})
      }
     }
    }
   }
   try {
    scene.render().close();delay(30)
    scene.render().use{
     val png=it.encodeToData(EncodedImageFormat.PNG)!!.bytes
     Files.write(root.resolve("панель-${width}-${if(dark)"тёмная"else"светлая"}.png"),png)
     val pixels=javax.imageio.ImageIO.read(java.io.ByteArrayInputStream(png))
     val white=(pixels.height-45 until pixels.height-16).sumOf{y->(16 until pixels.width-16).count{x->(pixels.getRGB(x,y)and 0xFFFFFF)==0xFFFFFF}}
     check(white>25){"Статус нижней панели не отрисован белым: $white"}
    }
    val measured=checkNotNull(layout)
    check(!measured.hasVisualOverflow)
    check(measured.getLineEnd(measured.lineCount-1)==measured.layoutInput.text.length)
   }finally{scene.close()}
  }
  // Полный экран с устаревшей сырой ошибкой: проверяется новый показ, не миграция БД.
  val failed=ProxyEntity(groupId=group).putBean(SOCKSBean().applyDefaultValues().apply{name="Недоступный профиль";serverAddress="127.0.0.1";serverPort=1081})
  failed.status=2;failed.ping=-1;failed.error="SOCKS server general failure";SagerDatabase.proxyDao.addProxy(failed)
  DataStore.selectedProxy=entity.id;LinuxRuntime.status.value=LinuxRuntime.Status(phase="Подключено",profile=entity.id)
  val scene=ImageComposeScene(458,650,Density(1f)){App()}
  try{
   scene.render(0L).close();delay(50)
   scene.render(500_000_000L).use{Files.write(root.resolve("главный-экран-458.png"),it.encodeToData(EncodedImageFormat.PNG)!!.bytes)}
   LinuxRuntime.currentConnectionTest.value=CurrentConnectionTest(profile=entity.id,latency=143)
   // У программной сцены время кадра по умолчанию нулевое: анимации продвигаем явно.
   val before=javax.imageio.ImageIO.read(root.resolve("главный-экран-458.png").toFile())
   // Не привязываем StateFlow/recompose к 50 мс: в эмуляции и на слабых ARM кадр дольше.
   var frame=1_000_000_000L
   withTimeout(15000){while(true){
    delay(150)
    scene.render(frame).use{Files.write(root.resolve("главный-экран-результат-458.png"),it.encodeToData(EncodedImageFormat.PNG)!!.bytes)}
    frame+=500_000_000L
    val after=javax.imageio.ImageIO.read(root.resolve("главный-экран-результат-458.png").toFile())
    val changed=(before.height-42 until before.height-12).sumOf{y->(12 until before.width-12).count{x->before.getRGB(x,y)!=after.getRGB(x,y)}}
    if(changed>100)break
   }}
   unchanged()
  }finally{scene.close()}
  for(frame in listOf(0f,.25f,.5f)){
   val scene=ImageComposeScene(240,160,Density(1f)){
    MaterialTheme(typography=androidTypography()){
     Column(Modifier.fillMaxSize(),verticalArrangement=Arrangement.Bottom){AndroidConnectionBar("Подключение…",0,0,CurrentConnectionTest(),"",Color(0xFFDE8670),Color(0xFFD65B37),Color(0xFFFFCCBC),{},{},ringFrame=frame)}
    }
   }
   try{scene.render().use{Files.write(root.resolve("кольцо-${(frame*100).toInt()}.png"),it.encodeToData(EncodedImageFormat.PNG)!!.bytes)}}finally{scene.close()}
  }
  println("Android-регрессии: локальные SOCKS/HTTP успех и отказ; нижняя проверка отдельно от БД; отмена и повтор; ошибки Android; все UID/PID; белый текст без обрезки 320/360/458, две темы; выемка и фазы кольца — успешно")
 }
}
