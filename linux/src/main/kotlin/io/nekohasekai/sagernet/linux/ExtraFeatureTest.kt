package io.nekohasekai.sagernet.linux
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.ktx.applyDefaultValues
import java.nio.file.*
import java.nio.ByteBuffer
import java.util.concurrent.*
import kotlinx.coroutines.*
import javax.swing.*
import javafx.application.Platform
import javafx.scene.Scene
import javafx.scene.web.WebView
import javafx.embed.swing.JFXPanel

object ExtraFeatureTest {
 fun run(){
  require(LinuxDatabase.directory.fileName.toString().startsWith("qa-")){"Нужна отдельная QA-база"}
  DataStore.initGlobal();val group=DataStore.currentGroupId();val bean=io.nekohasekai.sagernet.fmt.socks.SOCKSBean().applyDefaultValues().apply{name="Проверка резервирования";serverAddress="127.0.0.1";serverPort=1080}
  val entity=ProxyEntity(groupId=group).putBean(bean);SagerDatabase.proxyDao.addProxy(entity);DataStore.configurationStore.putString("qaBackup","значение")
  val before=LinuxBackup.export(true,true,true);val copy=LinuxBackup.parse(before);val oldCount=SagerDatabase.proxyDao.getAll().size
  SagerDatabase.proxyDao.addProxy(ProxyEntity(groupId=group).putBean(bean.clone()));DataStore.configurationStore.putString("qaBackup","другое")
  LinuxBackup.restore(copy,true,true,true);check(SagerDatabase.proxyDao.getAll().size==oldCount);check(DataStore.configurationStore.getString("qaBackup")=="значение")
  val hash=LinuxBackup.export(true,true,true);check(runCatching{LinuxBackup.parse("{\"version\":1,\"profiles\":[]}")}.isFailure);check(hash==LinuxBackup.export(true,true,true))
  LinuxBackup.resetSettings();check(SagerDatabase.proxyDao.getAll().size==oldCount);check(DataStore.configurationStore.getString("qaBackup")==null)
  println("Резервирование, транзакционное восстановление, отказ от Android Parcel без изменения данных и безопасный сброс: успешно")
  check(FirefoxFrames.frames.size==30&&FirefoxFrames.duration==1050);check(FirefoxFrames.frameAt(34)==0&&FirefoxFrames.frameAt(35)==1&&FirefoxFrames.frameAt(1049)==29&&FirefoxFrames.frameAt(1050)==0)
  for((from,to)in AndroidServicePaths.animations.values.flatMap{it.values})for(i in 0..20)androidx.compose.ui.graphics.vector.PathParser().parsePathString(AndroidServicePaths.interpolate(from,to,i/20f)).toPath()
  println("30 исходных кадров Firefox, 35 мс на кадр, цикл 1050 мс и все четыре исходных перехода кнопки: успешно")
  val id=ByteArray(12){it.toByte()};val ip=byteArrayOf(203.toByte(),0,113,9);val mask=byteArrayOf(0x21,0x12,0xA4.toByte(),0x42)
  val response=ByteBuffer.allocate(32).putShort(0x0101).putShort(12).putInt(0x2112A442).put(id).putShort(0x0020).putShort(8).put(0).put(1).putShort((45678 xor 0x2112).toShort()).put(ByteArray(4){(ip[it].toInt()xor mask[it].toInt()).toByte()}).array()
  val parsed=LinuxStun.decode(response,id)!!;check(parsed.mapped!!.address.hostAddress=="203.0.113.9"&&parsed.mapped.port==45678);check(LinuxStun.decode(response,ByteArray(12))==null)
  println("Декодирование XOR-MAPPED-ADDRESS, порт и проверка STUN transaction ID: успешно")
  val html=AndroidDashboard.asset("index.html")!!;check(String(html).contains("yacd"));val url=AndroidDashboard.url();val connection=java.net.URL(url).openConnection()as java.net.HttpURLConnection;check(connection.responseCode==200);check(String(connection.inputStream.readBytes()).contains("linux-websocket.js"));connection.disconnect();check(java.net.URI(url+"linux-websocket.js").toURL().readText().contains("window.WebSocket=LinuxWebSocket"));AndroidDashboard.stop()
  println("Панель: исходная Android yacd.zip, локальная раздача без внешнего сайта: успешно")
 }
 fun webview(){
  require(LinuxDatabase.directory.fileName.toString().startsWith("qa-"));Files.createDirectories(LinuxDatabase.directory)
  DataStore.initGlobal();DataStore.serviceMode="proxy";DataStore.mixedPort=io.nekohasekai.sagernet.ktx.mkPort()
  val profile=ProxyEntity(groupId=DataStore.currentGroupId()).putBean(io.nekohasekai.sagernet.fmt.socks.SOCKSBean().applyDefaultValues().apply{serverAddress="127.0.0.1";serverPort=9})
  profile.id=SagerDatabase.proxyDao.addProxy(profile);LinuxRuntime.connect(profile)
  runBlocking{withTimeout(15000){while(LinuxRuntime.status.value.phase!="Подключено"){check(LinuxRuntime.status.value.error.isBlank()){LinuxRuntime.status.value.error};delay(100)}}}
  val completed=CountDownLatch(1);var error:Throwable?=null;var frame:JFrame?=null
  try{
   SwingUtilities.invokeAndWait{
    val panel=JFXPanel();LinuxWebView.started=true;frame=JFrame("Проверка исходной панели NekoBox").apply{contentPane.add(panel);setSize(800,680);isVisible=true}
    Platform.runLater{try{
     Platform.setImplicitExit(false);val view=WebView();panel.scene=Scene(view)
     view.engine.loadWorker.stateProperty().addListener{_,_,state->if(state==javafx.concurrent.Worker.State.SUCCEEDED){
      check(view.engine.executeScript("String(WebSocket)").toString().contains("LinuxWebSocket")){"Платформенный транспорт панели не загружен"}
      view.engine.executeScript("window.__qaTraffic=0;const q=new URLSearchParams(location.search);window.__qaSocket=new WebSocket('ws://127.0.0.1:'+q.get('port')+'/traffic?token='+q.get('secret'));window.__qaSocket.onmessage=function(){window.__qaTraffic++}")
      // Проверяем готовность DOM и настоящего потока, а не фиксированную задержку: QEMU медленнее ПК.
      val deadline=System.nanoTime()+TimeUnit.SECONDS.toNanos(100)
      val poll=javafx.animation.Timeline()
      poll.keyFrames.add(javafx.animation.KeyFrame(javafx.util.Duration.seconds(0.5),javafx.event.EventHandler {
       try{
        val children=view.engine.executeScript("document.getElementById('app').children.length")as Number
        val text=view.engine.executeScript("document.body.innerText").toString()
        val messages=(view.engine.executeScript("window.__qaTraffic")as Number).toInt()
        if((children.toInt()==0||text.length<=40||messages==0)&&System.nanoTime()<deadline)return@EventHandler
        check(children.toInt()>0){"JavaScript панели не отрисовал приложение"}
        check(text.length>40){"Пустая панель WebKit: символов=${text.length}"}
        check(messages>0){"Панель не получила настоящий WebSocket-поток ядра"}
        println("WebKit: исходная панель отрисована, элементов корня=${children.toInt()}, символов текста=${text.length}, сообщений WebSocket=$messages")
        val image=view.snapshot(null,null);val pixels=image.pixelReader;val out=java.awt.image.BufferedImage(image.width.toInt(),image.height.toInt(),java.awt.image.BufferedImage.TYPE_INT_ARGB)
        for(y in 0 until out.height)for(x in 0 until out.width)out.setRGB(x,y,pixels.getArgb(x,y))
        javax.imageio.ImageIO.write(out,"png",LinuxDatabase.directory.resolve("панель-WebKit.png").toFile())
        poll.stop();completed.countDown()
       }catch(t:Throwable){error=t;poll.stop();completed.countDown()}
      }))
      poll.cycleCount=javafx.animation.Animation.INDEFINITE;poll.play()
     }else if(state==javafx.concurrent.Worker.State.FAILED){error=view.engine.loadWorker.exception;completed.countDown()}}
     view.engine.load(LinuxRuntime.dashboardURL(DataStore.yacdURL))
    }catch(t:Throwable){error=t;completed.countDown()}}
   }
   check(completed.await(150,TimeUnit.SECONDS)){"WebKit не завершил проверку за 150 секунд"};error?.let{throw it}
  }finally{SwingUtilities.invokeAndWait{frame?.dispose()};AndroidDashboard.stop();runBlocking{LinuxRuntime.shutdown()}}
 }
}
