package io.nekohasekai.sagernet.linux
import io.nekohasekai.sagernet.database.*
import moe.matsuri.nb4a.utils.JavaUtil.gson
import org.json.*
import java.nio.file.*
import java.nio.file.attribute.PosixFilePermissions

/** Linux-сериализация: никаких Android Parcel и неоднозначного импорта их байтов. */
object LinuxBackup {
 fun export(profiles:Boolean,rules:Boolean,settings:Boolean):String=JSONObject().apply{
  put("format","nekobox-ytm-linux");put("version",1)
  if(profiles){put("profiles",JSONArray(gson.toJson(SagerDatabase.proxyDao.getAll())));put("groups",JSONArray(gson.toJson(SagerDatabase.groupDao.allGroups())))}
  if(rules)put("rules",JSONArray(gson.toJson(SagerDatabase.rulesDao.allRules())))
  if(settings)put("settings",JSONObject(LinuxDatabase.preferences("settings")))
 }.toString(2)
 fun parse(text:String):JSONObject{
  require(text.toByteArray().size<=16*1024*1024){"Резервная копия слишком велика"}
  val j=JSONObject(text);require(j.optString("format")=="nekobox-ytm-linux"&&j.optInt("version")==1){"Это не резервная копия Linux-порта. Android Parcel пока не поддерживается; данные не изменены."};return j
 }
 fun write(path:Path,text:String){
  if(!Files.exists(path))Files.createFile(path,PosixFilePermissions.asFileAttribute(PosixFilePermissions.fromString("rw-------")))
  Files.setPosixFilePermissions(path,PosixFilePermissions.fromString("rw-------"));Files.writeString(path,text)
 }
 fun restore(j:JSONObject,profiles:Boolean,rules:Boolean,settings:Boolean){
  require(LinuxRuntime.status.value.phase=="Отключено"){"Перед восстановлением отключите VPN"}
  val ps=if(profiles&&j.has("profiles"))gson.fromJson(j.getJSONArray("profiles").toString(),Array<ProxyEntity>::class.java).toList()else null
  val gs=if(ps!=null)gson.fromJson(j.getJSONArray("groups").toString(),Array<ProxyGroup>::class.java).toList()else null
  val rs=if(rules&&j.has("rules"))gson.fromJson(j.getJSONArray("rules").toString(),Array<RuleEntity>::class.java).toList()else null
  ps?.forEach{require(it.id>0&&gs!!.any{g->g.id==it.groupId}){"Нарушены связи профилей и групп"};it.requireBean()}
  require(ps==null||ps.map{it.id}.distinct().size==ps.size);require(gs==null||gs.map{it.id}.distinct().size==gs.size)
  write(LinuxDatabase.directory.resolve("до-восстановления-${System.currentTimeMillis()}.json"),export(true,true,true))
  SagerDatabase.runInTransaction{
   if(ps!=null){SagerDatabase.proxyDao.reset();SagerDatabase.groupDao.reset();gs!!.forEach{SagerDatabase.groupDao.createGroup(it)};ps.forEach{SagerDatabase.proxyDao.addProxy(it)}}
   if(rs!=null){SagerDatabase.rulesDao.reset();rs.forEach{SagerDatabase.rulesDao.createRule(it)}}
   if(settings&&j.has("settings")){val config=j.getJSONObject("settings");LinuxDatabase.clearPreferences("settings");config.keys().forEach{DataStore.configurationStore.putString(it,config.getString(it))}}
   DataStore.currentGroupId()
  }
 }
 fun resetSettings(){require(LinuxRuntime.status.value.phase=="Отключено"){"Сначала отключите VPN"};write(LinuxDatabase.directory.resolve("до-сброса-${System.currentTimeMillis()}.json"),export(true,true,true));LinuxDatabase.clearPreferences("settings");DataStore.initGlobal();DataStore.currentGroupId()}
}
