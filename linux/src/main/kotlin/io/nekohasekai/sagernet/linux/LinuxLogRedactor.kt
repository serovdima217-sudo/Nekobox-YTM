package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.fmt.AbstractBean
import moe.matsuri.nb4a.utils.JavaUtil.gson
import org.json.*

/** Плагины не должны помещать пароли, UUID и закрытые ключи в журнал приложения. */
class LinuxLogRedactor(beans:List<AbstractBean>) {
 private val secrets=linkedSetOf<String>()
 private val sensitive=listOf("password","passwd","uuid","token","secret","privatekey","authpayload","preshared")
 init{beans.forEach{runCatching{inspect(JSONObject(gson.toJson(it)))}}}
 private fun inspect(value:Any?) {
  when(value){
   is JSONObject->for(key in value.keys()){
    val item=value.opt(key)
    if(item is String&&sensitive.any{key.lowercase().replace("_","").contains(it)}&&item.isNotBlank()){
     secrets.add(item);secrets.add(JSONObject.quote(item).removeSurrounding("\""));if(item.contains("-----BEGIN"))secrets.addAll(item.lines().filter{it.length>20&&!it.startsWith("-----")})
    }
    inspect(item)
   }
   is JSONArray->for(i in 0 until value.length())inspect(value.opt(i))
   is String->if(value.trimStart().startsWith("{"))runCatching{inspect(JSONObject(value))}
  }
 }
 fun redact(line:String):String {
  var safe=line
  for(secret in secrets.sortedByDescending{it.length})safe=if(secret.length>=4)safe.replace(secret,"[секрет скрыт]")else safe.replace(Regex("(?<![\\p{L}\\p{N}])"+Regex.escape(secret)+"(?![\\p{L}\\p{N}])"),"[секрет скрыт]")
  return safe.replace(Regex("[a-zA-Z][a-zA-Z0-9+.-]*://[^\\s]+"),"[ссылка скрыта]")
 }
}
