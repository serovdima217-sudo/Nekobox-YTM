package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.*
import io.nekohasekai.sagernet.fmt.internal.ChainBean
import io.nekohasekai.sagernet.ktx.applyDefaultValues
import moe.matsuri.nb4a.proxy.config.ConfigBean
import moe.matsuri.nb4a.utils.JavaUtil.gson
import moe.matsuri.nb4a.utils.Util
import org.json.*

object LinuxSharing {
 fun subscription(text:String):ProxyGroup? {
  val value=text.trim()
  val plain=runCatching{java.net.URI(value)}.getOrNull()
  if(plain?.scheme in listOf("http","https")&&plain?.host!=null&&plain.userInfo==null&&(!plain.rawQuery.isNullOrEmpty()||!plain.path.isNullOrEmpty()&&plain.path!="/"))return ProxyGroup(name="Подписка",type=1,subscription=SubscriptionBean().applyDefaultValues().apply{link=value})
  val group=when {
   value.startsWith("sn://subscription?")->KryoConverters.deserialize(ProxyGroup().apply{export=true},Util.zlibDecompress(Util.b64Decode(value.substringAfter('?')))).apply{export=false;id=0;ungrouped=false}
   value.startsWith("clash://install-config?")-> {
    val query=java.net.URI(value).rawQuery.split('&').associate{val p=it.split('=',limit=2);java.net.URLDecoder.decode(p[0],"UTF-8") to java.net.URLDecoder.decode(p.getOrElse(1){""},"UTF-8")}
    ProxyGroup(name=query["name"].orEmpty(),type=1,subscription=SubscriptionBean().applyDefaultValues().apply{link=query["url"].orEmpty()})
   }
   else->return null
  }
  val url=java.net.URI(group.subscription?.link?:error("В QR-коде нет ссылки подписки"))
  require(url.scheme in listOf("https","http")&&url.host!=null){"Недопустимая ссылка подписки"};group.type=1
  return group
 }
 fun config(entity:ProxyEntity):Pair<String,String> {
  val copy=entity.copy().putBean(entity.requireBean().clone())
  val bean=copy.requireBean()
  return if(bean is ConfigBean&&bean.type==2)bean.config to "SplitYTM.json" else copy.exportConfig()
 }
 fun duplicateIds(profiles:List<ProxyEntity>):List<ProxyEntity> {
  val seen=mutableSetOf<String>()
  return profiles.filter{profile->val bean=profile.requireBean().clone();bean.name="";!seen.add(bean.toUniversalLink())}
 }
 fun exportGroup(group:Long):String {
  val profiles=SagerDatabase.proxyDao.getByGroup(group)
  if(profiles.none{it.requireBean()is ChainBean})return profiles.joinToString("\n"){it.toStdLink()}
  profiles.forEach{validateChain(it)}
  val all=profiles.associateBy{it.id}.toMutableMap();val visited=mutableSetOf<Long>()
  fun add(entity:ProxyEntity,stack:Set<Long>){
   require(entity.id !in stack){"В цепочке есть цикл"};if(!visited.add(entity.id))return
   require(all.size<=10000){"Слишком много профилей в группе"}
   (entity.requireBean()as?ChainBean)?.proxies?.forEach{id->val p=all[id]?:SagerDatabase.proxyDao.getById(id)?:error("В цепочке отсутствует профиль $id");all[id]=p;add(p,stack+entity.id)}
  }
  profiles.forEach{add(it,emptySet())}
  return JSONObject().put("format","nekobox-ytm-group").put("version",1).put("profiles",JSONArray(gson.toJson(all.values))).toString(2)
 }
 fun importBundle(text:String,group:Long):Int? {
  if(!text.trimStart().startsWith("{"))return null
  val json=runCatching{JSONObject(text)}.getOrNull()?:return null
  if(json.optString("format")!="nekobox-ytm-group")return null
  require(json.optInt("version")==1){"Неизвестная версия группы"}
  val profiles=gson.fromJson(json.getJSONArray("profiles").toString(),Array<ProxyEntity>::class.java).toList()
  require(profiles.size in 1..10000&&profiles.map{it.id}.distinct().size==profiles.size&&profiles.all{it.id>0}){"Некорректная группа"}
  val all=profiles.associateBy{it.id}
  fun check(entity:ProxyEntity,stack:Set<Long>){require(stack.size<32&&entity.id !in stack){"Цикл или слишком длинная цепочка"};(entity.requireBean()as?ChainBean)?.proxies?.forEach{check(all[it]?:error("Не найдена часть цепочки"),stack+entity.id)}}
  profiles.forEach{it.requireBean().initializeDefaultValues();require((it.requireBean()as?ChainBean)?.proxies?.isEmpty()!=true){"Пустая цепочка"}}
  profiles.forEach{check(it,emptySet())}
  SagerDatabase.runInTransaction{
   val ids=mutableMapOf<Long,Long>();var order=SagerDatabase.proxyDao.nextOrder(group)?:0
   profiles.forEach{p->val old=p.id;p.id=0;p.groupId=group;p.userOrder=order++;ids[old]=SagerDatabase.proxyDao.addProxy(p)}
   profiles.forEach{p->(p.requireBean()as?ChainBean)?.let{b->b.proxies=b.proxies.map{ids.getValue(it)};SagerDatabase.proxyDao.updateProxy(p)}}
  }
  return profiles.size
 }
 fun validateChain(entity:ProxyEntity){
  fun visit(p:ProxyEntity,stack:Set<Long>){
   val bean=p.requireBean()as?ChainBean?:return
   require(stack.size<32&&(p.id==0L||p.id !in stack)){"В цепочке обнаружен цикл"}
   require(bean.proxies.isNotEmpty()){"Добавьте профили в цепочку"}
   bean.proxies.forEach{id->require(id!=entity.id||id==0L){"Цепочка не может ссылаться на себя"};visit(SagerDatabase.proxyDao.getById(id)?:error("Часть цепочки удалена"),stack+p.id)}
  }
  visit(entity,emptySet())
 }
}
