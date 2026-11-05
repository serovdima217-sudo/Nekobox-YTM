@file:OptIn(androidx.compose.material.ExperimentalMaterialApi::class)
package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.*
import kotlinx.coroutines.*
import io.nekohasekai.sagernet.linux.EntityEditors.initLinux
import io.nekohasekai.sagernet.linux.EntityEditors.serializeLinux
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.fmt.*
import io.nekohasekai.sagernet.ktx.*
import moe.matsuri.nb4a.proxy.config.ConfigBean
import moe.matsuri.nb4a.utils.JavaUtil.gson
import org.json.JSONArray
import java.awt.Toolkit
import java.awt.datatransfer.*
import java.io.File

fun main(args:Array<String>) {
 if(args.contains("--plugin-network-test")){PluginNetworkTest.run(args[args.indexOf("--plugin-network-test")+1]);return}
 if(args.contains("--remaining-self-test")){RemainderTest.run();return}
 if(args.contains("--android-ui-regression-test")){AndroidUiRegressionTest.run();return}
 if(args.contains("--process-bypass-ui-test")){ProcessBypassUiTest.run();return}
 if(args.contains("--process-bypass-self-test")){ProcessBypassTest.run();return}
 if(args.contains("--tray-self-test")){DesktopTray.test();return}
 if(args.contains("--extra-test")){ExtraFeatureTest.run();return}
 if(args.contains("--webview-self-test")){ExtraFeatureTest.webview();return}
 if(args.contains("--self-test")){SelfTest.run();return}
 if(args.contains("--import-subscription-file")) {
  val file=File(args[args.indexOf("--import-subscription-file")+1]);val config=org.json.JSONObject(file.readText())
  DataStore.initGlobal();val link=config.getString("url");require(java.net.URI(link).scheme in listOf("https","http"))
  val existing=SagerDatabase.groupDao.allGroups().find{it.subscription?.link==link}
  val group=existing?:ProxyGroup(name=config.getString("name"),type=1,subscription=SubscriptionBean().applyDefaultValues().apply{this.link=link})
  if(group.id==0L)group.id=SagerDatabase.groupDao.createGroup(group)
  val count=runBlocking{LinuxProfiles.update(group,config.optString("content_file").takeIf{it.isNotBlank()}?.let{File(it).readText()})};DataStore.selectedGroup=group.id
  val profiles=SagerDatabase.proxyDao.getByGroup(group.id);DataStore.selectedProxy=(profiles.firstOrNull{(it.requireBean() as? ConfigBean)?.type==2}?:profiles.firstOrNull())?.id?:0
  println("Подписка импортирована: $count профилей; группа выбрана, секретная ссылка не выводится")
  return
 }
 if(args.contains("--import-file")) {
  val i=args.indexOf("--import-file");DataStore.initGlobal();val group=DataStore.currentGroupId()
  val count=runBlocking{LinuxProfiles.importText(File(args[i+1]).readText(),group)}
  DataStore.selectedProxy=SagerDatabase.proxyDao.getByGroup(group).first().id
  println("Импортировано профилей: $count");return
 }
 if(args.contains("--live-test")){val i=args.indexOf("--live-test");LiveTest.run(args[i+1],args.contains("--tun"));return}
 if(args.contains("--ui-test")){UiTest.run();return}
 if(args.contains("--window-self-test"))require(System.getenv("NEKOBOX_LINUX_STATE")?.let{File(it).name.startsWith("qa-")}==true){"Нужна отдельная QA-база"}
 DataStore.initGlobal();DataStore.currentGroupId()
 Runtime.getRuntime().addShutdownHook(Thread{runBlocking{LinuxRuntime.shutdown()}})
 application {
  val scope=rememberCoroutineScope()
  LaunchedEffect(Unit){if(!args.contains("--window-self-test")){LinuxSubscriptions.start(scope);if(DataStore.persistAcrossReboot&&DataStore.configurationStore.getBoolean("linuxWasConnected"))SagerDatabase.proxyDao.getById(DataStore.currentProfile)?.let{LinuxRuntime.connect(it)}}}
  var visible by remember{mutableStateOf(true)}
  val state=rememberWindowState(width=480.dp,height=820.dp)
  var hostWindow by remember{mutableStateOf<java.awt.Window?>(null)}
  fun finish(){scope.launch{LinuxRuntime.shutdown();AndroidDashboard.stop();exitApplication()}}
  val tray=remember{DesktopTray({scope.launch{visible=true;state.isMinimized=false;hostWindow?.toFront();hostWindow?.requestFocus()}},{scope.launch{if(LinuxRuntime.status.value.phase!="Отключено")LinuxRuntime.disconnect()else SagerDatabase.proxyDao.getById(DataStore.selectedProxy)?.let{LinuxRuntime.connect(it)}}},{finish()},{scope.launch{visible=!visible;if(visible){state.isMinimized=false;hostWindow?.toFront();hostWindow?.requestFocus()}}})}
  DisposableEffect(Unit){runCatching{tray.install()}.onFailure{System.err.println("Не удалось установить трей: "+it.javaClass.simpleName)};onDispose{tray.close()}}
  LaunchedEffect(Unit){while(true){tray.update(LinuxRuntime.status.value.phase,DataStore.selectedProxy>0);delay(500)}}
  if(args.contains("--window-self-test"))LaunchedEffect(Unit){
   try{
    DataStore.serviceMode="proxy";DataStore.mixedPort=mkPort();val bean=io.nekohasekai.sagernet.fmt.socks.SOCKSBean().applyDefaultValues().apply{serverAddress="127.0.0.1";serverPort=9}
    val profile=ProxyEntity(groupId=DataStore.currentGroupId()).putBean(bean);profile.id=SagerDatabase.proxyDao.addProxy(profile);DataStore.selectedProxy=profile.id;LinuxRuntime.connect(profile)
    withTimeout(15000){while(hostWindow==null||!tray.ready||LinuxRuntime.status.value.phase!="Подключено"){check(LinuxRuntime.status.value.error.isBlank()){LinuxRuntime.status.value.error};delay(100)}}
    delay(1200);hostWindow!!.dispatchEvent(java.awt.event.WindowEvent(hostWindow,java.awt.event.WindowEvent.WINDOW_CLOSING));delay(800)
    check(!visible&&!hostWindow!!.isVisible){"Закрытие не скрыло окно"};check(LinuxRuntime.status.value.phase=="Подключено"){"Скрытие окна остановило службу"}
    tray.openAction();delay(800);check(visible&&hostWindow!!.isVisible){"Трей не вернул окно"};check(LinuxRuntime.status.value.phase=="Подключено")
    tray.mouseAction(java.awt.event.MouseEvent.BUTTON1);delay(500);check(!visible&&!hostWindow!!.isVisible);check(LinuxRuntime.status.value.phase=="Подключено")
    tray.mouseAction(java.awt.event.MouseEvent.BUTTON1);delay(500);check(visible&&hostWindow!!.isVisible);check(LinuxRuntime.status.value.phase=="Подключено")
    tray.mouseAction(java.awt.event.MouseEvent.BUTTON3);delay(300);check(visible&&hostWindow!!.isVisible)
    println("ЛКМ: окно скрывается и возвращается без остановки ядра; ПКМ не меняет видимость")
    println("Окно скрыто в трей без остановки настоящего ядра, возвращено через действие трея; выход завершает службу");finish()
   }catch(t:Throwable){t.printStackTrace();LinuxRuntime.shutdown();tray.close();kotlin.system.exitProcess(1)}
  }
  val icon=remember{androidx.compose.ui.graphics.painter.BitmapPainter(androidx.compose.ui.res.loadImageBitmap(object{}.javaClass.getResourceAsStream("/android/icon/nekobox.png")!!))}
  Window(onCloseRequest={if(tray.ready)visible=false else finish()},title="Nekobox YTM",state=state,visible=visible,icon=icon) {
   hostWindow=window
   App()
  }
 }
}
fun copyText(text:String)=Toolkit.getDefaultToolkit().systemClipboard.setContents(StringSelection(text),null)
fun pasteText():String=runCatching{Toolkit.getDefaultToolkit().systemClipboard.getData(DataFlavor.stringFlavor)as String}.getOrDefault("")
fun bytes(n:Long):String = when {n>=1073741824->"%.2f ГиБ".format(n/1073741824.0);n>=1048576->"%.2f МиБ".format(n/1048576.0);n>=1024->"%.1f КиБ".format(n/1024.0);else->"$n Б"}

@Composable fun SelectionContainerCompat(text:String,modifier:Modifier){androidx.compose.foundation.text.selection.SelectionContainer(modifier.verticalScroll(rememberScrollState()).padding(12.dp)){Text(text,fontFamily=androidx.compose.ui.text.font.FontFamily.Monospace,fontSize=12.sp)}}
@Composable fun TextDialog(title:String,initial:String,hint:String,dismiss:()->Unit,save:(String)->Unit){var text by remember{mutableStateOf(initial)};AlertDialog(onDismissRequest=dismiss,title={Text(title)},text={OutlinedTextField(text,{text=it},modifier=Modifier.fillMaxWidth().heightIn(min=150.dp,max=400.dp),label={Text(hint)})},confirmButton={TextButton(onClick={save(text)}){Text("Сохранить")}},dismissButton={TextButton(onClick=dismiss){Text("Отмена")}})}
@Composable fun SubscriptionDialog(dismiss:()->Unit,save:(String,String)->Unit){var name by remember{mutableStateOf("")};var url by remember{mutableStateOf("")};AlertDialog(onDismissRequest=dismiss,title={Text("Подписка")},text={Column{OutlinedTextField(name,{name=it},label={Text("Название")});OutlinedTextField(url,{url=it},label={Text("Ссылка")});Text("Свободная основная и резервная выбираются автоматически",Modifier.padding(top=12.dp),style=MaterialTheme.typography.caption)}},confirmButton={TextButton(onClick={save(name.ifBlank{"Подписка"},url)}){Text("Добавить")}},dismissButton={TextButton(onClick=dismiss){Text("Отмена")}})}

@Composable fun EditorDialog(entity:ProxyEntity,dismiss:()->Unit,save:()->Unit)=AndroidProfileEditor(entity,dismiss,save)
fun protocolChoices():List<Pair<String,()->AbstractBean>> = listOf(
 "SplitYTM" to {ConfigBean().apply{initializeDefaultValues();type=2}},
 "SOCKS" to {io.nekohasekai.sagernet.fmt.socks.SOCKSBean()},
 "HTTP" to {io.nekohasekai.sagernet.fmt.http.HttpBean()},
 "Shadowsocks" to {io.nekohasekai.sagernet.fmt.shadowsocks.ShadowsocksBean()},
 "VMess" to {io.nekohasekai.sagernet.fmt.v2ray.VMessBean()},
 "VLESS" to {io.nekohasekai.sagernet.fmt.v2ray.VMessBean().apply{initializeDefaultValues();alterId=-1}},
 "Trojan" to {io.nekohasekai.sagernet.fmt.trojan.TrojanBean()},
 "Hysteria 2" to {io.nekohasekai.sagernet.fmt.hysteria.HysteriaBean().apply{initializeDefaultValues();protocolVersion=2}},
 "TUIC" to {io.nekohasekai.sagernet.fmt.tuic.TuicBean()},
 "SSH" to {io.nekohasekai.sagernet.fmt.ssh.SSHBean()},
 "WireGuard" to {io.nekohasekai.sagernet.fmt.wireguard.WireGuardBean()},
 "AnyTLS" to {moe.matsuri.nb4a.proxy.anytls.AnyTLSBean()},
 "ShadowTLS" to {moe.matsuri.nb4a.proxy.shadowtls.ShadowTLSBean()},
 "Naïve" to {io.nekohasekai.sagernet.fmt.naive.NaiveBean()},
 "Mieru" to {io.nekohasekai.sagernet.fmt.mieru.MieruBean()},
 "Trojan-Go" to {io.nekohasekai.sagernet.fmt.trojan_go.TrojanGoBean()},
 "Цепочка" to {io.nekohasekai.sagernet.fmt.internal.ChainBean()},
 "Полная конфигурация sing-box" to {ConfigBean()}
)
@Composable fun RulesScreen(modifier:Modifier,revision:Int,refresh:()->Unit,message:(String)->Unit) {
 val rules=remember(revision){SagerDatabase.rulesDao.allRules()};var edit by remember{mutableStateOf<RuleEntity?>(null)}
 Column(modifier.fillMaxWidth()){
  TextButton(onClick={edit=RuleEntity(enabled=true)}){Text("+ Правило")}
  LazyColumn{items(rules){rule->ListItem(Modifier.clickable{edit=rule.copy()},text={Text(rule.displayName())},secondaryText={Text(rule.mkSummary())},trailing={Switch(rule.enabled,{rule.enabled=it;SagerDatabase.rulesDao.updateRule(rule);refresh()})});Divider()}}
 }
 edit?.let{rule->EntityDialog("Правило маршрутизации",{DataStore.editingId=rule.id;rule.initLinux()},"route_preferences",{edit=null},{rule.serializeLinux();if(rule.id==0L)SagerDatabase.rulesDao.createRule(rule)else SagerDatabase.rulesDao.updateRule(rule);edit=null;refresh()})}
}
@Composable fun EntityDialog(title:String,load:()->Unit,screen:String,dismiss:()->Unit,save:()->Unit) {
 remember(screen){load();true};var error by remember{mutableStateOf("")}
 DialogWindow(onCloseRequest=dismiss,title=title,state=rememberDialogState(width=560.dp,height=720.dp)) {
  MaterialTheme{Surface(Modifier.fillMaxSize()){Column{
   TopAppBar(title={Text(title)},navigationIcon={TextButton(onClick=dismiss){Text("‹",color=contentColorFor(MaterialTheme.colors.primarySurface),fontSize=26.sp)}},actions={TextButton(onClick={try{save()}catch(t:Throwable){error=t.message?:"Ошибка"}}){Text("Сохранить",color=contentColorFor(MaterialTheme.colors.primarySurface))}})
   PreferenceScreen(screen,DataStore.profileCacheStore,Modifier.weight(1f))
   if(error.isNotBlank())Text(error,Modifier.padding(12.dp),color=MaterialTheme.colors.error)
  }}}
 }
}
@Composable fun OutboundItem(pref:PreferenceSpec) {
 var dialog by remember{mutableStateOf(false)};var revision by remember{mutableIntStateOf(0)}
 val profiles=remember{SagerDatabase.proxyDao.getAll()}
 val routes=listOf(0L to "Прокси",-1L to "Напрямую",-2L to "Блокировать")+profiles.map{it.id to it.displayName()}
 val stored=DataStore.profileCacheStore.getString(pref.key,"0")!!.toIntOrNull()?:0
 val selected=when(pref.key){"groupFrontProxy"->DataStore.frontProxy;"groupLandingProxy"->DataStore.landingProxy;else->when(stored){0->0L;1->-1L;2->-2L;else->DataStore.routeOutboundRule}}
 ListItem(Modifier.clickable{dialog=true},text={Text(pref.title)},secondaryText={Text(routes.find{it.first==selected}?.second?:"Не выбран")})
 if(dialog)AlertDialog(onDismissRequest={dialog=false},title={Text(pref.title)},text={Column(Modifier.heightIn(max=500.dp).verticalScroll(rememberScrollState())){routes.forEach{(id,name)->TextButton(onClick={
  when(pref.key){"groupFrontProxy"->{DataStore.frontProxy=id;DataStore.frontProxyTmp=if(id>0)3 else 0};"groupLandingProxy"->{DataStore.landingProxy=id;DataStore.landingProxyTmp=if(id>0)3 else 0};else->{DataStore.routeOutbound=when(id){0L->0;-1L->1;-2L->2;else->3};DataStore.routeOutboundRule=id}}
  dialog=false;revision++
 }){Text(name)}}}},confirmButton={TextButton(onClick={dialog=false}){Text("Отмена")}})
}
