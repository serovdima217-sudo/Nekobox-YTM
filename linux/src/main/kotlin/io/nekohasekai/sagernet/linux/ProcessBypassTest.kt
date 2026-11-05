package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.*
import io.nekohasekai.sagernet.fmt.socks.SOCKSBean
import io.nekohasekai.sagernet.ktx.applyDefaultValues
import org.json.*
import java.nio.file.*

object ProcessBypassTest {
 fun run() {
  val root=Path.of(System.getenv("NEKOBOX_LINUX_STATE")?:error("Нужна QA-база"))
  require(root.fileName.toString().startsWith("qa-"))
  Files.createDirectories(root)
  val curl=root.resolve("selected/curl").toString();val python=root.resolve("selected/python3").toString()
  val savedEnabled=DataStore.proxyApps;val savedPaths=LinuxProcessBypass.selected();val savedMode=DataStore.serviceMode
  try {
   check(LinuxProcessBypass.normalize(listOf(curl,curl,"com.android.example","relative","/bad\u0000path")).size==1)
   LinuxProcessBypass.save(listOf(curl,python),true)
   check(LinuxProcessBypass.selected()==listOf(curl,python).sorted()&&DataStore.proxyApps)
   val current=Files.readSymbolicLink(Path.of("/proc",ProcessHandle.current().pid().toString(),"exe")).toString()
   check(LinuxProcessBypass.active().any{it.path==current&&ProcessHandle.current().pid()in it.pids})
   val fixtures=listOf(LinuxActiveProcess("/usr/bin/firefox","firefox",listOf(100,101)),LinuxActiveProcess("/usr/bin/curl","curl",listOf(102)))
   check(LinuxProcessBypass.entries(fixtures,setOf(curl),"selected").single().pids.isEmpty())
   check(LinuxProcessBypass.entries(fixtures,emptySet(),"101").single().name=="firefox")
   check(LinuxProcessBypass.entries(fixtures,emptySet(),"FIREFOX").single().pids.size==2)
   val custom=JSONObject().put("domain_suffix",JSONArray().put("example.org")).put("outbound","direct")
   val config=JSONObject().put("route",JSONObject().put("rules",JSONArray().put(custom)))
   var previous=LinuxProcessBypass.apply(config,LinuxProcessBypass.settings())
   check(config.getJSONObject("route").getJSONArray("rules").length()==2)
   previous=LinuxProcessBypass.apply(config,LinuxProcessBypass.Settings(true,listOf(curl)),previous)
   check(config.getJSONObject("route").getJSONArray("rules").length()==2)
   LinuxProcessBypass.apply(config,LinuxProcessBypass.Settings(false,listOf(curl)),previous)
   check(config.getJSONObject("route").getJSONArray("rules").length()==1)
   check(config.getJSONObject("route").getJSONArray("rules").getJSONObject(0).similar(custom))
   val empty=JSONObject();LinuxProcessBypass.apply(empty,LinuxProcessBypass.Settings(true,emptyList()))
   check(empty.getJSONObject("route").getJSONArray("rules").length()==0)
   DataStore.serviceMode="vpn"
   val bean=SOCKSBean().applyDefaultValues().apply{serverAddress="127.0.0.1";serverPort=1080}
   val entity=ProxyEntity(groupId=DataStore.currentGroupId()).putBean(bean)
   val generated=LinuxRuntime.linuxConfig(entity,22880,true,0,"")
   val rule=generated.getJSONObject("route").getJSONArray("rules").getJSONObject(0)
   check(rule.getJSONArray("process_path").length()==2&&rule.getString("outbound")=="direct")
   check(!rule.has("user_id")&&!rule.has("process_name"))
   check(rule.getBoolean("process_path_include_children"))
   check(generated.getJSONObject("route").getBoolean("find_process"))
   val nonTun=LinuxRuntime.linuxConfig(entity,22881,false,0,"")
   check((0 until nonTun.getJSONObject("route").getJSONArray("rules").length()).none{nonTun.getJSONObject("route").getJSONArray("rules").optJSONObject(it)?.has("process_path")==true})
   Files.writeString(root.resolve("process-bypass-generated.json"),generated.toString(2))
   Files.writeString(root.resolve("process-bypass-rule.json"),rule.toString(2))
   Files.writeString(root.resolve("process-bypass-expected.json"),JSONObject().put("selected_curl",curl).put("selected_python",python).toString())
   println("Исключения процессов: /proc, поиск, PID, сохранение, точные пути, пустой выбор, замена и отключение правил — успешно")
  }finally{LinuxProcessBypass.save(savedPaths,savedEnabled);DataStore.serviceMode=savedMode}
 }
}
