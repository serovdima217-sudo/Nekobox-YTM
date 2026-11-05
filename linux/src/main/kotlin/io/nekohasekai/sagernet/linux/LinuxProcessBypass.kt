package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.DataStore
import io.nekohasekai.sagernet.fmt.TAG_DIRECT
import org.json.JSONArray
import org.json.JSONObject
import java.nio.file.Files
import java.nio.file.Path

/** PID используются только для отображения. Правило привязано к исполняемому файлу. */
data class LinuxActiveProcess(val path:String,val name:String,val pids:List<Long>,val uid:Long=-1) {
 val selectable:Boolean get()=path.startsWith("/")&&uid!=0L
 val identity:String get()=path+"#"+pids.joinToString(",")
}

object LinuxProcessBypass {
 const val KEY="linuxBypassExecutables"
 data class Settings(val enabled:Boolean,val paths:List<String>) {
  fun rule():JSONObject? = if(!enabled||paths.isEmpty())null else JSONObject()
   .put("process_path",JSONArray(paths)).put("process_path_include_children",true).put("action","route").put("outbound",TAG_DIRECT)
 }
 fun normalize(paths:Collection<String>):List<String> = paths.filter {
  it.startsWith("/") && it.length<=8192 && it.none{c->c=='\u0000'||c=='\n'||c=='\r'}
 }.map{Path.of(it).normalize().toString()}.distinct().sorted()
 fun selected():List<String> = runCatching {
  val array=JSONArray(DataStore.configurationStore.getString(KEY,"[]"))
  normalize((0 until array.length()).map{array.getString(it)})
 }.getOrDefault(emptyList())
 fun settings()=Settings(DataStore.proxyApps,selected())
 fun save(paths:Collection<String>,enabled:Boolean) {
  DataStore.configurationStore.putString(KEY,JSONArray(normalize(paths)).toString())
  DataStore.bypass=true
  DataStore.proxyApps=enabled
 }
 /** /proc перечисляется целиком: чужие UID и недоступные exe больше не пропадают. */
 fun active():List<LinuxActiveProcess> {
  val records=mutableListOf<LinuxActiveProcess>()
  Files.list(Path.of("/proc")).use{stream->stream.filter{it.fileName.toString().toLongOrNull()!=null}.forEach{proc->
   runCatching {
    val pid=proc.fileName.toString().toLong()
    val name=runCatching{Files.readString(proc.resolve("comm")).trim()}.getOrDefault("Процесс $pid")
    val uid=runCatching{Files.readAllLines(proc.resolve("status")).first{it.startsWith("Uid:")}.substringAfter(":").trim().split(Regex("\\s+"))[0].toLong()}.getOrDefault(-1)
    val path=runCatching{Files.readSymbolicLink(proc.resolve("exe")).toString().removeSuffix(" (deleted)")}.getOrDefault("@proc/$pid")
    if(Files.exists(proc))records+=LinuxActiveProcess(path,name,listOf(pid),uid)
   }
  }}
  return records.sortedWith(compareBy<LinuxActiveProcess>{it.name.lowercase()}.thenBy{it.pids.firstOrNull()?:0})
 }
 /** Разовый read-only helper: не запускает, не останавливает и не перенастраивает VPN. */
 suspend fun privileged():List<LinuxActiveProcess> = kotlinx.coroutines.runInterruptible(kotlinx.coroutines.Dispatchers.IO) {
  val output=Files.createTempFile("nekobox-процессы-",".json")
  val errors=Files.createTempFile("nekobox-процессы-",".log")
  var process:Process?=null
  try {
   process=ProcessBuilder("pkexec",LinuxRuntime.binary("linux-supervisor").toString(),"--list-processes")
    .redirectOutput(output.toFile()).redirectError(errors.toFile()).start()
   check(process.waitFor(120,java.util.concurrent.TimeUnit.SECONDS)){"Истекло время ожидания прав администратора"}
   check(process.exitValue()==0){"Доступ не предоставлен. Доступные процессы остались в списке."}
   require(Files.size(output)<=8*1024*1024){"Слишком большой ответ списка процессов"}
   parseProcesses(Files.readString(output))
  }finally{process?.let{if(it.isAlive)it.destroyForcibly()};Files.deleteIfExists(output);Files.deleteIfExists(errors)}
 }
 fun parseProcesses(text:String):List<LinuxActiveProcess> {
  val records=JSONArray(text)
  return (0 until records.length()).mapNotNull {i->
   val row=records.getJSONObject(i);val pid=row.optLong("pid")
   if(pid<=0)null else LinuxActiveProcess(row.optString("path").takeIf{it.startsWith("/")}?:"@proc/$pid",row.optString("name","Процесс $pid"),listOf(pid),row.optLong("uid",-1))
  }
 }
 fun entries(active:List<LinuxActiveProcess>,selected:Set<String>,query:String):List<LinuxActiveProcess> {
  val all=active.toMutableList()
  selected.filter{path->active.none{it.path==path}}.forEach{path->all+=LinuxActiveProcess(path,Path.of(path).fileName.toString(),emptyList())}
  val q=query.trim()
  return all.filter{q.isEmpty()||it.name.contains(q,true)||it.path.contains(q,true)||it.pids.any{pid->pid.toString().contains(q)}||"UID: ${it.uid}".contains(q,true)}
   .sortedWith(compareBy<LinuxActiveProcess>{it.path !in selected}.thenBy{it.name.lowercase()}.thenBy{it.path}.thenBy{it.pids.firstOrNull()?:0})
 }
 /** Удаляем только собственное прежнее правило, не затрагивая пользовательские маршруты. */
 fun apply(config:JSONObject,settings:Settings,previousRule:String?=null):String? {
  val route=config.optJSONObject("route")?:JSONObject().also{config.put("route",it)}
  val old=route.optJSONArray("rules")?:JSONArray()
  val rules=JSONArray();val next=settings.rule()
  if(next!=null){
   val outbounds=config.optJSONArray("outbounds")?:JSONArray().also{config.put("outbounds",it)}
   val direct=(0 until outbounds.length()).mapNotNull{outbounds.optJSONObject(it)}.find{it.optString("tag")==TAG_DIRECT}
   require(direct==null||direct.optString("type")=="direct"){"В профиле тег direct занят другим выходом"}
   if(direct==null)outbounds.put(JSONObject().put("type","direct").put("tag",TAG_DIRECT))
  }
  if(next!=null)rules.put(next)
  var removed=false
  for(i in 0 until old.length()) {
   val rule=old.get(i)
   if(!removed&&previousRule!=null&&rule is JSONObject&&rule.similar(JSONObject(previousRule))){removed=true;continue}
   rules.put(rule)
  }
  route.put("rules",rules).put("find_process",true)
  return next?.toString()
 }
}
