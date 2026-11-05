package io.nekohasekai.sagernet.linux

import io.nekohasekai.sagernet.fmt.*
import io.nekohasekai.sagernet.fmt.mieru.*
import io.nekohasekai.sagernet.fmt.naive.*
import io.nekohasekai.sagernet.fmt.trojan_go.*
import io.nekohasekai.sagernet.fmt.hysteria.*
import io.nekohasekai.sagernet.ktx.mkPort
import moe.matsuri.nb4a.proxy.config.ConfigBean
import org.json.*
import java.nio.file.*
import java.nio.file.attribute.PosixFilePermissions

/** Оригинальные Android-генераторы + отдельные процессы Linux, не APK-плагины. */
object LinuxPlugins {
 data class Plan(val entries:JSONArray,val ports:List<Int>,val needsRoot:Boolean)
 fun write(directory:Path,name:String,text:String):Path=directory.resolve(name).also{Files.writeString(it,text);Files.setPosixFilePermissions(it,PosixFilePermissions.fromString("rw-------"))}
 fun prepare(build:ConfigBuildResult,directory:Path):Plan {
  val entries=JSONArray();val ports=mutableListOf<Int>();var root=false
  for(index in build.externalIndex)for((port,entity)in index.chain){
   val bean=entity.requireBean();var name="";var args=listOf<String>();val env=JSONObject()
   when(bean){
    is NaiveBean->{name="naive-plugin";args=listOf(write(directory,"naive-$port.json",bean.buildNaiveConfig(port)).toString())}
    is MieruBean->{
     name="mieru-plugin"
     val config=JSONObject(bean.buildMieruConfig(port)).put("rpcPort",mkPort()).put("socks5ListenLAN",false)
     val file=write(directory,"mieru-$port.json",config.toString())
     val binary=LinuxRuntime.binary(name)
     val compiled=directory.resolve("mieru-$port.pb")
     val report=write(directory,"mieru-$port-check.log","")
     val builder=ProcessBuilder(binary.toString(),"apply","config",file.toString()).redirectErrorStream(true).redirectOutput(report.toFile())
     builder.environment()["MIERU_CONFIG_FILE"]=compiled.toString();builder.environment()["HOME"]=directory.toString()
     val compiler=builder.start()
     try{check(compiler.waitFor(10,java.util.concurrent.TimeUnit.SECONDS)){"Подготовка Mieru не завершилась вовремя"};check(compiler.exitValue()==0){"Mieru отклонил конфигурацию: "+Files.readString(report).replace(bean.password,"[пароль скрыт]").take(500)}}finally{if(compiler.isAlive)compiler.destroyForcibly()}
     Files.setPosixFilePermissions(compiled,PosixFilePermissions.fromString("rw-------"))
     env.put("MIERU_CONFIG_FILE",compiled.toString());args=listOf("run")
    }
    is TrojanGoBean->{name="trojan-go-plugin";args=listOf("-config",write(directory,"trojan-go-$port.json",bean.buildTrojanGoConfig(port)).toString())}
    is HysteriaBean->{
     require(bean.protocolVersion==1){"Для Hysteria 2 выберите UDP: этот транспорт обслуживается встроенным ядром"}
     name="hysteria-plugin";root=root||bean.protocol==HysteriaBean.PROTOCOL_FAKETCP
     args=listOf("--no-check","--log-level","warn","--config",write(directory,"hysteria-$port.json",bean.buildHysteria1Config(port){write(directory,"hysteria-$port.ca","").toFile()}).toString(),"client")
    }
    is ConfigBean->{
     require(bean.type==2){"Неизвестный внешний формат конфигурации"}
     name="ytm-client";val config=JSONObject(bean.config).put("listen","127.0.0.1:$port")
     LinuxRuntime.physicalInterface()?.let{config.put("bind_interface",it)}
     args=listOf("-config",write(directory,"split-$port.json",config.toString()).toString())
    }
    else->error("Нет Linux-исполнителя для ${entity.displayType()}")
   }
   val binary=LinuxRuntime.binary(name)
   require(Files.isExecutable(binary)){"В AppImage отсутствует Linux-плагин $name"}
   entries.put(JSONObject().put("path",binary.toString()).put("args",JSONArray(args)).put("env",env));ports+=port
  }
  return Plan(entries,ports,root)
 }
}
