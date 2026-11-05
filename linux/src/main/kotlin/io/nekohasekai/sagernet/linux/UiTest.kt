@file:OptIn(androidx.compose.ui.ExperimentalComposeUiApi::class)
package io.nekohasekai.sagernet.linux
import androidx.compose.ui.ImageComposeScene
import androidx.compose.ui.unit.Density
import androidx.compose.material.*
import androidx.compose.foundation.layout.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.PointerEventType
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.unit.dp
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.ktx.applyDefaultValues
import java.nio.file.*
import org.jetbrains.skia.EncodedImageFormat

object UiTest {
 fun run() {
  val state=System.getenv("NEKOBOX_LINUX_STATE")?:error("Нужна QA-база")
  require(Path.of(state).fileName.toString().startsWith("qa-"))
  DataStore.initGlobal();val group=DataStore.currentGroupId()
  if(SagerDatabase.proxyDao.getAll().isEmpty()) {
   for((name,factory)in protocolChoices().take(5)){val bean=factory().applyDefaultValues();bean.name="Профиль $name";SagerDatabase.proxyDao.addProxy(ProxyEntity(groupId=group).putBean(bean))}
  }
  require(AndroidMenus.load("main_drawer_menu").map{it.id}==listOf("nav_configuration","nav_group","nav_route","nav_settings","nav_logcat","nav_traffic","nav_tools","nav_tuiguang","nav_faq","nav_about")){"Нарушен порядок Android-меню"}
  require(AndroidXml.dimension("layout_profile","selected_view","layout_width",-1f)==4f)
  val standard=AndroidPreferences.screen("standard_v2ray_preferences")
  require(standard.first{it.key=="uuid"}.icon=="ic_baseline_person_24")
  require(standard.first{it.key=="uuid"}.attributes["dialogLayout"]=="@layout/layout_password_dialog")
  require(standard.first{it.key=="sni"}.parents.contains("serverSecurityCategory"))
  val http=ProxyEntity(groupId=group).putBean(io.nekohasekai.sagernet.fmt.http.HttpBean().applyDefaultValues());LinuxProfiles.editorLoad(http)
  val httpKeys=androidVisiblePreferences("standard_v2ray_preferences",DataStore.profileCacheStore,http).map{it.key}
  require("username" in httpKeys&&"password" in httpKeys&&"uuid" !in httpKeys&&"alterId" !in httpKeys)
  println("Порядок меню, полоса 4dp, значки, парольный диалог, иерархия и видимость HTTP: успешно")
  val out=Path.of(state).resolve("изображения");Files.createDirectories(out)
  fun render(name:String,content: @androidx.compose.runtime.Composable () -> Unit) {
   val scene=ImageComposeScene(if(name=="портрет-480")480 else 820,820,Density(1f),content=content)
   try {scene.render().close();val image=scene.render();image.use {Files.write(out.resolve("$name.png"),it.encodeToData(EncodedImageFormat.PNG)!!.bytes)}}finally{scene.close()}
  }
  for((i,pane)in listOf("Конфигурации","Меню","Настройки","Темы","Группы","Правила маршрутизации","Журнал","Инструменты","Резервная копия","STUN","Ресурсы","О программе","Панель sing-box").withIndex()) {render("экран-$i"){App(pane)};println("Экран $pane: отрисован")}
  for(i in 1..21){DataStore.appTheme=i;render("тема-$i"){App()}}
  DataStore.appTheme=2
  for((name,factory)in protocolChoices()) {
   val entity=ProxyEntity(groupId=group).putBean(factory().applyDefaultValues());LinuxProfiles.editorLoad(entity)
   render("редактор-${entity.type}-$name".replace('/','_')){App(initialEditor=entity)}
  }
  for(frame in listOf(0,7,15,23,29))render("Firefox-кадр-$frame"){MaterialTheme(typography=androidTypography()){Surface{FirefoxLoading("Установка соединения",vertical=true,frameOverride=frame)}}}
  render("портрет-480"){App()}
  val scene=ImageComposeScene(480,820,Density(1f)){App()}
  try {scene.render().close();scene.sendPointerEvent(PointerEventType.Press,Offset(24f,28f));scene.sendPointerEvent(PointerEventType.Release,Offset(24f,28f));scene.render().close();scene.render().use{Files.write(out.resolve("меню-после-нажатия.png"),it.encodeToData(EncodedImageFormat.PNG)!!.bytes)}}finally{scene.close()}
  val saved=LinuxRuntime.status.value;DataStore.nightTheme=2;DataStore.appTheme=2
  try{LinuxRuntime.status.value=LinuxRuntime.Status(phase="Подключено");render("подключено-светлая-тема"){App()}}finally{LinuxRuntime.status.value=saved}
  AndroidDashboard.stop()
  println("Проверены экраны, 21 тема, редакторы XML и реальное нажатие меню. Изображения: $out")
 }
}
