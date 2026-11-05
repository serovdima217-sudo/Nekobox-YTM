package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.AbstractBean
import io.nekohasekai.sagernet.fmt.toUniversalLink
import io.nekohasekai.sagernet.group.RawUpdater
import io.nekohasekai.sagernet.ktx.applyDefaultValues
import moe.matsuri.nb4a.proxy.config.ConfigBean
import moe.matsuri.nb4a.proxy.tmsplit.*
import moe.matsuri.nb4a.utils.JavaUtil.gson
import kotlinx.coroutines.*
import okhttp3.*
import org.json.*
import java.util.concurrent.TimeUnit

object LinuxProfiles {
 suspend fun parse(text:String):List<AbstractBean> {require(text.toByteArray().size<=16*1024*1024){"Слишком большой импорт"};return RawUpdater.parseRaw(text)?.onEach{it.initializeDefaultValues()}?.takeIf{it.isNotEmpty()}?:error("В данных не найдены профили")}
 suspend fun importText(text:String,group:Long):Int {
  require(text.toByteArray().size<=16*1024*1024){"Слишком большой импорт"}
  LinuxSharing.importBundle(text,group)?.let{return it}
  val beans=parse(text)
  SagerDatabase.runInTransaction {
   var order=SagerDatabase.proxyDao.nextOrder(group)?:0
   for(bean in beans)SagerDatabase.proxyDao.addProxy(ProxyEntity(groupId=group,userOrder=order++).putBean(bean))
  }
  return beans.size
 }
 suspend fun update(group:ProxyGroup,providedContent:String?=null):Int = withContext(Dispatchers.IO) {
  val subscription=group.subscription?:error("Группа не является подпиской")
  val inline=subscription.link.trimStart().startsWith("{")||subscription.link.contains("proxies:")
  val content=providedContent?:if(inline)subscription.link else run {
  val uri=java.net.URI(subscription.link)
  require(uri.scheme in listOf("http","https")){"Подписка должна использовать HTTP или HTTPS"}
  val builder=OkHttpClient.Builder().connectTimeout(20,TimeUnit.SECONDS).readTimeout(30,TimeUnit.SECONDS).followRedirects(true)
  if(subscription.updateWhenConnectedOnly){val address=LinuxRuntime.activeProxy()?:error("Для этой подписки сначала подключите VPN или прокси");builder.proxy(java.net.Proxy(java.net.Proxy.Type.SOCKS,address))}
  val client=builder.build()
  val req=Request.Builder().url(subscription.link).header("User-Agent",subscription.customUserAgent.ifBlank{"NekoBox/Linux/1.4.2 (Prefer ClashMeta Format)"}).build()
  try{val response=client.newCall(req).execute()
  response.use {
   require(it.isSuccessful){"Сервер подписки вернул HTTP ${it.code}"}
   require((it.body?.contentLength()?:0)<16*1024*1024){"Подписка слишком велика"}
   val bytes=it.body?.byteStream()?.readNBytes(16*1024*1024+1)?:error("Подписка пуста")
   require(bytes.size<=16*1024*1024){"Подписка слишком велика"}
   subscription.subscriptionUserinfo=it.header("subscription-userinfo")?:""
   bytes.toString(Charsets.UTF_8)
  }}finally{client.dispatcher.executorService.shutdownNow();client.connectionPool.evictAll()}
  }
  var beans=parse(content)
  if(subscription.forceResolve)for(bean in beans){
   if(bean.serverAddress.isNotBlank()&&!bean.serverAddress.matches(Regex("[0-9.:]+"))){
    val host=bean.serverAddress
    val addresses=java.net.InetAddress.getAllByName(host)
    val address=(addresses.firstOrNull{it.address.size==4}?:addresses.first()).hostAddress
    when(bean){
     is io.nekohasekai.sagernet.fmt.v2ray.StandardV2RayBean->{if(bean.sni.isBlank())bean.sni=host;if(bean.host.isBlank()&&bean.type in listOf("ws","http","httpupgrade"))bean.host=host}
     is io.nekohasekai.sagernet.fmt.hysteria.HysteriaBean->if(bean.sni.isBlank())bean.sni=host
     is io.nekohasekai.sagernet.fmt.trojan_go.TrojanGoBean->if(bean.sni.isBlank())bean.sni=host
     is io.nekohasekai.sagernet.fmt.naive.NaiveBean->if(bean.sni.isBlank())bean.sni=host
     is moe.matsuri.nb4a.proxy.anytls.AnyTLSBean->if(bean.sni.isBlank())bean.sni=host
    }
    bean.serverAddress=address
   }
  }
  if(subscription.deduplication)beans=beans.distinctBy{bean->bean.clone().apply{name=""}.toUniversalLink()}
  val names=mutableMapOf<String,Int>()
  for(bean in beans){val original=bean.displayName();val n=names.getOrDefault(original,0)+1;names[original]=n;if(n>1)bean.name="$original ($n)"}
  val old=SagerDatabase.proxyDao.getByGroup(group.id)
  val byName=old.associateBy{it.displayName()}
  SagerDatabase.runInTransaction {
   val keep=mutableSetOf<Long>()
   beans.forEachIndexed { index,bean ->
    val entity=byName[bean.displayName()]?.copy()?:ProxyEntity(groupId=group.id)
    entity.putBean(bean);entity.userOrder=index.toLong()
    if(entity.id==0L)entity.id=SagerDatabase.proxyDao.addProxy(entity)else SagerDatabase.proxyDao.updateProxy(entity)
    keep.add(entity.id)
   }
   SagerDatabase.proxyDao.deleteProxy(old.filter{it.id !in keep})
   subscription.lastUpdated=(System.currentTimeMillis()/1000).toInt()
   SagerDatabase.groupDao.updateGroup(group)
  }
  beans.size
 }
 fun editorLoad(entity:ProxyEntity):LinuxEditor? {
  val bean=entity.requireBean()
  if(bean is ConfigBean){DataStore.profileCacheStore.putString("profileName",bean.name);DataStore.profileCacheStore.putString("serverConfig",bean.config);DataStore.profileCacheStore.putBoolean("isOutboundOnly",bean.type==1);DataStore.profileCacheStore.putString("tmSplitLinks",if(bean.type==2)bean.tmSplitLinks() else "");return null}
  return editorFor(bean)?.also{it.load(bean)}
 }
 fun editorSave(entity:ProxyEntity,editor:LinuxEditor?) {
  val bean=entity.requireBean()
  if(bean is ConfigBean) {
   bean.name=DataStore.profileCacheStore.getString("profileName",bean.name)
   if(bean.type==2) {
    val text=DataStore.profileCacheStore.getString("tmSplitLinks","")!!.trim()
    val parsed=if(text.startsWith("tm-split://")||text.startsWith("{"))parseTmSplit(text) else parseTmSplit(JSONObject().put("type","tm-split").put("name",bean.name).put("links",JSONArray(text.split(Regex("[\\s,;]+")).filter{it.isNotBlank()})).toString())
    bean.config=JSONObject(parsed.config).put("name",bean.name).toString()
   } else {bean.type=if(DataStore.profileCacheStore.getBoolean("isOutboundOnly"))1 else 0;bean.config=DataStore.profileCacheStore.getString("serverConfig","")!!;JSONObject(bean.config)}
  } else editor?.save(bean)?:error("Неизвестный тип редактора")
  bean.initializeDefaultValues()
  if(bean is io.nekohasekai.sagernet.fmt.internal.ChainBean)LinuxSharing.validateChain(entity)
  if(bean !is ConfigBean&&bean !is io.nekohasekai.sagernet.fmt.internal.ChainBean) {require(bean.serverAddress.isNotBlank()){ "Укажите адрес сервера" };require(bean.serverPort in 1..65535){"Порт должен быть от 1 до 65535"}}
  if(entity.id==0L){entity.userOrder=SagerDatabase.proxyDao.nextOrder(entity.groupId)?:0;entity.id=SagerDatabase.proxyDao.addProxy(entity)}else SagerDatabase.proxyDao.updateProxy(entity)
 }
}
