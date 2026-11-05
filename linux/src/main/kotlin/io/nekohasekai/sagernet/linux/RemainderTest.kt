@file:OptIn(androidx.compose.ui.ExperimentalComposeUiApi::class)
package io.nekohasekai.sagernet.linux

import androidx.compose.foundation.layout.*
import androidx.compose.material.*
import androidx.compose.ui.ImageComposeScene
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.*
import io.nekohasekai.sagernet.fmt.internal.ChainBean
import io.nekohasekai.sagernet.fmt.socks.SOCKSBean
import io.nekohasekai.sagernet.ktx.*
import kotlinx.coroutines.*
import moe.matsuri.nb4a.proxy.tmsplit.awaitTmSplitSocks
import org.jetbrains.skia.EncodedImageFormat
import org.json.JSONObject
import java.awt.image.BufferedImage
import java.nio.file.*

/** Только новая QA-база, тестовые пароли и отдельные дочерние процессы; TUN не используется. */
object RemainderTest {
 fun run()=runBlocking {
  val directory=Path.of(System.getenv("NEKOBOX_LINUX_STATE")?:error("Нужна QA-база"));require(directory.fileName.toString().startsWith("qa-"))
  require(SagerDatabase.proxyDao.getAll().isEmpty());DataStore.initGlobal();DataStore.serviceMode="proxy";DataStore.mixedPort=mkPort()
  val group=DataStore.currentGroupId()
  fun socks(name:String,password:String)=ProxyEntity(groupId=group).putBean(SOCKSBean().applyDefaultValues().apply{this.name=name;serverAddress="127.0.0.1";serverPort=1080;username="тест";this.password=password})
  val first=socks("Профиль 🌐","пароль:+=&");first.id=SagerDatabase.proxyDao.addProxy(first)
  val second=socks("Другой пароль","другой");second.id=SagerDatabase.proxyDao.addProxy(second)
  val same=socks("Другое имя","пароль:+=&");same.id=SagerDatabase.proxyDao.addProxy(same)
  check(LinuxSharing.duplicateIds(listOf(first,second,same)).map{it.id}==listOf(same.id))
  val subscription=ProxyGroup(name="Подписка 🌐",type=1,subscription=SubscriptionBean().applyDefaultValues().apply{link="https://example.invalid/private?token=тест";customUserAgent="NekoBox QA";deduplication=true})
  val subscriptionLink=subscription.toUniversalLink()
  val restored=LinuxSharing.subscription(subscriptionLink)!!;check(restored.name==subscription.name&&restored.subscription!!.link==subscription.subscription!!.link&&restored.subscription!!.deduplication)
  for(text in listOf(first.toStdLink(),first.requireBean().toUniversalLink(),subscriptionLink,"tm-split://тест-кириллицы-🌐?данные=+=&")){
   val image=LinuxQr.encode(text);check(LinuxQr.decode(image)==listOf(text));check((image.getRGB(0,0)and 0xFFFFFF)==0xFFFFFF)
   val path=directory.resolve("проверка-qr.png");LinuxQr.save(path,image);check(LinuxQr.decode(LinuxQr.read(path))==listOf(text))
   val rotated=BufferedImage(image.height,image.width,BufferedImage.TYPE_INT_RGB)
   for(y in 0 until image.height)for(x in 0 until image.width)rotated.setRGB(image.height-1-y,x,image.getRGB(x,y))
   check(LinuxQr.decode(rotated)==listOf(text))
   for(y in 0 until image.height)for(x in 0 until image.width)rotated.setRGB(x,y,image.getRGB(x,y)xor 0xFFFFFF)
   check(LinuxQr.decode(rotated)==listOf(text))
  }
  check(runCatching{LinuxQr.encode("x".repeat(5000))}.isFailure)
  val image=LinuxQr.encode(first.requireBean().toUniversalLink())
  val scene=ImageComposeScene(360,550,Density(1f)){MaterialTheme(typography=androidTypography()){Surface{AndroidQrContent(image,"Профиль 🌐",Modifier.fillMaxSize().padding(16.dp))}}}
  try{scene.render(0).use{val png=it.encodeToData(EncodedImageFormat.PNG)!!.bytes;Files.write(directory.resolve("qr-окно-360.png"),png);check(LinuxQr.decode(javax.imageio.ImageIO.read(java.io.ByteArrayInputStream(png))).contains(first.requireBean().toUniversalLink()))}}finally{scene.close()}
  val before=SagerDatabase.proxyDao.getAll().size
  runCatching{SagerDatabase.runInTransaction{SagerDatabase.proxyDao.addProxy(socks("Отмена транзакции","QA"));error("Отмена")}}
  check(SagerDatabase.proxyDao.getAll().size==before){"Вложенная транзакция не откатилась"}
  val chain=ProxyEntity(groupId=group).putBean(ChainBean().applyDefaultValues().apply{name="Цепочка";proxies=listOf(first.id,second.id)})
  chain.id=SagerDatabase.proxyDao.addProxy(chain);LinuxSharing.validateChain(chain)
  val encoded=LinuxSharing.exportGroup(group);check(JSONObject(encoded).getString("format")=="nekobox-ytm-group")
  val importedGroup=ProxyGroup(name="Импорт цепочки");importedGroup.id=SagerDatabase.groupDao.createGroup(importedGroup)
  check(LinuxSharing.importBundle(encoded,importedGroup.id)==4)
  val imported=SagerDatabase.proxyDao.getByGroup(importedGroup.id);val importedIds=imported.map{it.id}.toSet()
  val importedChain=imported.single{it.type==ProxyEntity.TYPE_CHAIN};check((importedChain.requireBean()as ChainBean).proxies.all{it in importedIds})
  (chain.requireBean()as ChainBean).proxies=listOf(chain.id);SagerDatabase.proxyDao.updateProxy(chain);check(runCatching{LinuxSharing.validateChain(chain)}.isFailure)
  (chain.requireBean()as ChainBean).proxies=listOf(first.id,second.id);SagerDatabase.proxyDao.updateProxy(chain)
  LinuxRules.reset();check(SagerDatabase.rulesDao.allRules().size==9&&SagerDatabase.rulesDao.allRules().none{it.enabled})
  check(JSONObject(LinuxSharing.config(first).first).has("outbounds"))
  check(LinuxSharing.subscription("https://example.invalid/feed?token=QA")?.type==1)
  check(LinuxSharing.subscription("https://QA:QA@example.invalid:443")==null)
  val now=System.currentTimeMillis()/1000;subscription.subscription!!.apply{autoUpdate=true;autoUpdateDelay=10;lastUpdated=(now-599).toInt()}
  check(!LinuxSubscriptions.due(subscription,now,false));subscription.subscription!!.lastUpdated=(now-600).toInt();check(LinuxSubscriptions.due(subscription,now,false))
  subscription.subscription!!.updateWhenConnectedOnly=true;check(!LinuxSubscriptions.due(subscription,now,false)&&LinuxSubscriptions.due(subscription,now,true))
  val forceGroup=ProxyGroup(name="Разрешение имени",type=1,subscription=SubscriptionBean().applyDefaultValues().apply{link="https://example.invalid/feed";forceResolve=true;deduplication=true});forceGroup.id=SagerDatabase.groupDao.createGroup(forceGroup)
  val tls=moe.matsuri.nb4a.proxy.anytls.AnyTLSBean().applyDefaultValues().apply{name="TLS 1";serverAddress="localhost";serverPort=443;password="QA"}
  val secondLink=tls.clone().apply{name="TLS 2"}.toUniversalLink()
  check(LinuxProfiles.update(forceGroup,tls.toUniversalLink()+"\n"+secondLink)==1)
  val resolved=SagerDatabase.proxyDao.getByGroup(forceGroup.id).single().requireBean()as moe.matsuri.nb4a.proxy.anytls.AnyTLSBean
  check(resolved.serverAddress=="127.0.0.1"&&resolved.sni=="localhost")
  val sanitized=LinuxLogRedactor(listOf(tls)).redact("password=QA; https://example.invalid/private?token=QA")
  check(!sanitized.contains("password=QA")&&!sanitized.contains("example.invalid"))
  println("Подписки: часы обновления, только через активный прокси, дедупликация независимо от названия, разрешение localhost с сохранением TLS SNI — успешно")
  println("QR: генерация/PNG/чтение/поворот/инверсия/снимок интерфейса 360; Unicode и пароли; подписка; дедупликация; цепочки и перенос ID; отказ при цикле; откат SQLite; экспорт и сброс маршрутов — успешно")
  val choices=listOf("Naïve","Mieru","Trojan-Go","Hysteria 1")
  try{
   for(name in choices){
    val bean=protocolChoices().first{it.first==if(name=="Hysteria 1")"Hysteria 2"else name}.second().applyDefaultValues().apply{this.name="QA $name";serverAddress="127.0.0.1";serverPort=9}
    when(bean){
     is io.nekohasekai.sagernet.fmt.naive.NaiveBean->{bean.proto="https";bean.username="QA";bean.password="QA"}
     is io.nekohasekai.sagernet.fmt.mieru.MieruBean->{bean.protocol="TCP";bean.username="QA";bean.password="QA"}
     is io.nekohasekai.sagernet.fmt.trojan_go.TrojanGoBean->{bean.password="QA";bean.allowInsecure=true}
     is io.nekohasekai.sagernet.fmt.hysteria.HysteriaBean->{bean.protocolVersion=1;bean.protocol=io.nekohasekai.sagernet.fmt.hysteria.HysteriaBean.PROTOCOL_WECHAT_VIDEO;bean.serverPorts="9";bean.uploadMbps=10;bean.downloadMbps=50;bean.allowInsecure=true}
    }
    val entity=ProxyEntity(groupId=group).putBean(bean);entity.id=SagerDatabase.proxyDao.addProxy(entity)
    LinuxRuntime.connect(entity)
    withTimeout(15000){while(LinuxRuntime.status.value.phase!="Подключено"){check(LinuxRuntime.status.value.error.isBlank()){LinuxRuntime.status.value.error+"\n"+LinuxRuntime.logs.value};delay(100)}}
    java.net.Socket("127.0.0.1",DataStore.mixedPort).use{socket->socket.soTimeout=2000;socket.getOutputStream().write(byteArrayOf(5,1,0));check(socket.getInputStream().read()==5&&socket.getInputStream().read()==0)}
    val descendants=ProcessHandle.current().descendants().use{it.toList()}
    check(descendants.size>=3){"Нет отдельного процесса плагина"}
    LinuxRuntime.disconnect();withTimeout(20000){while(descendants.any{it.isAlive})delay(100)};LinuxRuntime.awaitStopped()
    println("Linux-плагин $name: настоящий процесс + sing-box, готовность SOCKS, отключение и уборка всех дочерних процессов — успешно")
   }
   val failureProfile=SagerDatabase.proxyDao.getByGroup(group).first{it.requireBean()is io.nekohasekai.sagernet.fmt.trojan_go.TrojanGoBean}
   LinuxRuntime.connect(failureProfile)
   withTimeout(15000){while(LinuxRuntime.status.value.phase!="Подключено"){check(LinuxRuntime.status.value.error.isBlank());delay(50)}}
   val children=ProcessHandle.current().descendants().use{it.toList()}
   // binfmt/QEMU показывает интерпретатор в /proc/exe; исходный ELF остаётся в argv.
    val pluginPath=LinuxRuntime.binary("trojan-go-plugin").toAbsolutePath().toString()
    check(children.single{child->val info=child.info();info.command().orElse("").endsWith("trojan-go-plugin")||info.arguments().orElse(emptyArray()).any{it==pluginPath}}.destroyForcibly())
   withTimeout(15000){while(LinuxRuntime.status.value.phase!="Отключено")delay(50)}
   LinuxRuntime.awaitStopped();check(children.none{it.isAlive});check(LinuxRuntime.status.value.error.isNotBlank())
   println("Аварийное завершение плагина: служба остановлена, ошибка показана, дочерние процессы не остались")
   val parentDir=Files.createDirectory(directory.resolve("проверка-родителя"));val port=mkPort()
   val built=buildConfig(failureProfile,forTest=true);val plan=LinuxPlugins.prepare(built,parentDir)
   val planFile=parentDir.resolve("plugins.json");Files.writeString(planFile,plan.entries.toString())
   val config=parentDir.resolve("core.json");Files.writeString(config,LinuxRuntime.linuxConfig(failureProfile,port,false,0,"QA",built=built).toString())
   val parent=ProcessBuilder("sleep","30").start()
   val helper=ProcessBuilder(LinuxRuntime.binary("linux-supervisor").toString(),"--parent",parent.pid().toString(),"--core",LinuxRuntime.binary("nekobox-core").toString(),"--config",config.toString(),"--plugins",planFile.toString()).redirectErrorStream(true).redirectOutput(parentDir.resolve("служба.log").toFile()).start()
   var childProcesses=emptyList<ProcessHandle>()
   try{
    awaitTmSplitSocks(listOf(port)+plan.ports,15000);childProcesses=helper.descendants().use{it.toList()};check(childProcesses.size==2)
    parent.destroy();check(parent.waitFor(5,java.util.concurrent.TimeUnit.SECONDS));check(helper.waitFor(15,java.util.concurrent.TimeUnit.SECONDS));check(childProcesses.none{it.isAlive})
    println("Исчезновение процесса интерфейса: отдельный supervisor сам завершил ядро и плагин")
   }finally{parent.destroyForcibly();if(helper.isAlive){helper.outputStream.write("Остановить\n".toByteArray());helper.outputStream.flush();helper.waitFor(15,java.util.concurrent.TimeUnit.SECONDS)};if(helper.isAlive)helper.destroyForcibly();childProcesses.filter{it.isAlive}.forEach{it.destroyForcibly()}}
  }finally{LinuxRuntime.shutdown()}
 }
}
