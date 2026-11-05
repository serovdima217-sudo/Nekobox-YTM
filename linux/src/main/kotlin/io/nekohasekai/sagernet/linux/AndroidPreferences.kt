package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.ktx.app
import org.json.*
import javax.xml.parsers.DocumentBuilderFactory
import org.w3c.dom.Element

/** Описания экранов и полей — исходные Android XML, а не новый набор desktop-настроек. */
data class PreferenceSpec(val key:String,val title:String,val summary:String,val type:String,val defaultValue:String,val entries:List<String>,val values:List<String>,val category:Boolean=false,val icon:String="",val parents:List<String> = emptyList(),val attributes:Map<String,String> = emptyMap())
object AndroidPreferences {
 private val arrays by lazy { JSONObject(javaClass.getResourceAsStream("/android/arrays.json")!!.reader().readText()) }
 fun text(value:String):String = if(value.startsWith("@string/")||value.startsWith("@android:string/"))app.getString(value.substringAfter('/')) else value
 fun array(value:String):List<String> {val a=arrays.optJSONArray(value.substringAfter('/'))?:return emptyList();return (0 until a.length()).map{text(a.getString(it))}}
 fun screen(name:String):List<PreferenceSpec> {
  val input=javaClass.getResourceAsStream("/android/xml/$name.xml")?:return emptyList()
  val factory=DocumentBuilderFactory.newInstance();factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl",true)
  val root=input.use { factory.newDocumentBuilder().parse(it).documentElement }
  fun attr(e:Element,key:String)=e.getAttribute("app:$key").ifEmpty{e.getAttribute("android:$key")}
  fun visit(e:Element,parents:List<String> = emptyList()):List<PreferenceSpec> {
   val key=attr(e,"key");val title=text(attr(e,"title"));val tag=e.tagName.substringAfterLast('.')
   val own=if(key.isNotEmpty()||tag=="PreferenceCategory")listOf(PreferenceSpec(key,title,text(attr(e,"summary")),tag,attr(e,"defaultValue"),array(attr(e,"entries")),array(attr(e,"entryValues")),tag=="PreferenceCategory",attr(e,"icon").substringAfterLast('/'),parents,(0 until e.attributes.length).associate{val a=e.attributes.item(it);a.nodeName.substringAfter(':') to a.nodeValue}))else emptyList()
   return own+(0 until e.childNodes.length).flatMap { i -> (e.childNodes.item(i)as?Element)?.let{visit(it,if(tag=="PreferenceCategory")parents+key.ifBlank{title}else parents)}?:emptyList() }
  }
  return visit(root)
 }
 fun editorScreen(entity:ProxyEntity):String = when(entity.type) {
  ProxyEntity.TYPE_SOCKS->"socks_preferences"
  ProxyEntity.TYPE_SS->"shadowsocks_preferences"
  ProxyEntity.TYPE_HYSTERIA->"hysteria_preferences"
  ProxyEntity.TYPE_SSH->"ssh_preferences"
  ProxyEntity.TYPE_WG->"wireguard_preferences"
  ProxyEntity.TYPE_TUIC->"tuic_preferences"
  ProxyEntity.TYPE_NAIVE->"naive_preferences"
  ProxyEntity.TYPE_MIERU->"mieru_preferences"
  ProxyEntity.TYPE_TROJAN_GO->"trojan_go_preferences"
  ProxyEntity.TYPE_ANYTLS->"anytls_preferences"
  ProxyEntity.TYPE_SHADOWTLS->"shadowtls_preferences"
  ProxyEntity.TYPE_CONFIG->if(entity.configBean?.type==2)"tm_split_preferences"else"config_preferences"
  ProxyEntity.TYPE_CHAIN->"balancer_preferences"
  else->"standard_v2ray_preferences"
 }
 fun get(store:PreferenceStore,pref:PreferenceSpec):String = store.getString(pref.key)?:pref.defaultValue
}
