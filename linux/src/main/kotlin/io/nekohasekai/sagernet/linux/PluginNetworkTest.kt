package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.ktx.*
import io.nekohasekai.sagernet.fmt.naive.NaiveBean
import io.nekohasekai.sagernet.fmt.mieru.MieruBean
import io.nekohasekai.sagernet.fmt.trojan_go.TrojanGoBean
import io.nekohasekai.sagernet.fmt.hysteria.HysteriaBean
import kotlinx.coroutines.*
import org.json.JSONObject
import java.nio.file.*

/** Проверка полного пути HTTP → sing-box → настоящий плагин → локальный тестовый сервер. */
object PluginNetworkTest {
 fun run(path:String)=runBlocking {
  val state=Path.of(System.getenv("NEKOBOX_LINUX_STATE")?:error("Нужна QA-база"));require(state.fileName.toString().startsWith("qa-")&&SagerDatabase.proxyDao.getAll().isEmpty())
  val config=JSONObject(Files.readString(Path.of(path)));DataStore.initGlobal();DataStore.serviceMode="proxy";DataStore.bypassLan=false;DataStore.bypassLanInCore=false
  try{
   val cases=config.getJSONArray("cases")
   for(i in 0 until cases.length()){
    val item=cases.getJSONObject(i);val type=item.getString("type")
    val choice=when(type){"naive"->"Naïve";"mieru"->"Mieru";"trojan-go"->"Trojan-Go";"hysteria1"->"Hysteria 2";else->error("Неизвестный тест")}
    val bean=protocolChoices().first{it.first==choice}.second().applyDefaultValues().apply{name="QA";serverAddress="127.0.0.1";serverPort=item.getInt("port")}
    when(bean){
     is NaiveBean->{bean.proto=item.optString("protocol","http");bean.username="QA";bean.password="QA-password"}
     is MieruBean->{bean.protocol=item.optString("protocol","TCP");bean.username="QA";bean.password="QA-password"}
     is TrojanGoBean->{bean.password="QA-password";bean.allowInsecure=true;bean.sni="localhost";if(item.optBoolean("websocket")){bean.type="ws";bean.host="localhost";bean.path="/qa"}}
     is HysteriaBean->{bean.protocolVersion=1;bean.protocol=HysteriaBean.PROTOCOL_WECHAT_VIDEO;bean.serverPorts=item.getInt("port").toString();bean.authPayloadType=HysteriaBean.TYPE_STRING;bean.authPayload="QA-password";bean.allowInsecure=true;bean.sni="localhost";bean.uploadMbps=10;bean.downloadMbps=50}
    }
    val entity=ProxyEntity(groupId=DataStore.currentGroupId()).putBean(bean);entity.id=SagerDatabase.proxyDao.addProxy(entity)
    DataStore.connectionTestURL=config.getString("target")+"/"+i
    val latency=LinuxRuntime.urlTest(entity);check(latency>=0)
    withTimeout(10000){while(ProcessHandle.current().descendants().use{it.anyMatch{p->p.isAlive}})delay(50)}
    println("Передача настоящего HTTP через ${item.getString("name")}, sing-box и тестовый сервер: $latency мс; все дочерние процессы остановлены")
   }
  }finally{LinuxRuntime.shutdown()}
 }
}
