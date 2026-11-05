@file:OptIn(androidx.compose.material.ExperimentalMaterialApi::class)
package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.foundation.*
import androidx.compose.animation.*
import androidx.compose.animation.core.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import kotlinx.coroutines.*
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.ktx.*
import io.nekohasekai.sagernet.fmt.*
import org.json.JSONArray
import io.nekohasekai.sagernet.linux.EntityEditors.initLinux
import io.nekohasekai.sagernet.linux.EntityEditors.serializeLinux
import androidx.compose.ui.input.key.*
import androidx.compose.ui.focus.*
import java.io.File

private fun string(key:String)=app.getString(key)
private val androidThemeNames=listOf("Pink_SSR","Red","Pink_SSR","Pink","Purple","DeepPurple","Indigo","Blue","LightBlue","Cyan","Teal","Green","LightGreen","Lime","Yellow","Amber","Orange","DeepOrange","Brown","Grey","BlueGrey","Black")
@Composable fun App(initialPane:String="Конфигурации",initialEditor:ProxyEntity?=null) {
 val updates by LinuxSubscriptions.revision.collectAsState()
 val scope=rememberCoroutineScope();val status by LinuxRuntime.status.collectAsState();val currentTest by LinuxRuntime.currentConnectionTest.collectAsState()
 val drawerItems=remember{AndroidMenus.load("main_drawer_menu").filter{it.id!="nav_traffic"}}
 var pane by remember{mutableStateOf(when(initialPane){"Группы"->"nav_group";"Правила маршрутизации"->"nav_route";"Настройки","Темы"->"nav_settings";"Журнал"->"nav_logcat";"Инструменты","Резервная копия"->"nav_tools";"Панель sing-box"->"nav_traffic";"STUN"->"stun";"Ресурсы"->"assets";"О программе"->"nav_about";else->"nav_configuration"})}
 var drawer by remember{mutableStateOf(initialPane=="Меню")}
 var revision by remember{mutableIntStateOf(0)};var themeRevision by remember{mutableIntStateOf(0)}
 var groupId by remember{mutableLongStateOf(DataStore.currentGroupId())}
 val groups=remember(revision){SagerDatabase.groupDao.allGroups()}
 val profiles=remember(revision,groupId){val ps=SagerDatabase.proxyDao.getByGroup(groupId);when(groups.find{it.id==groupId}?.order){1->ps.sortedBy{it.displayName().lowercase()};2->ps.sortedBy{if(it.ping>0)it.ping else Int.MAX_VALUE};else->ps}}
 var selected by remember{mutableLongStateOf(DataStore.selectedProxy)}
 var editor by remember{mutableStateOf<ProxyEntity?>(initialEditor)}
 var ruleEditor by remember{mutableStateOf<RuleEntity?>(null)}
 var groupEditor by remember{mutableStateOf<ProxyGroup?>(null)}
 var qr by remember{mutableStateOf<Pair<String,String>?>(null)}
 var scanner by remember{mutableStateOf(false)}
 var resetRoutes by remember{mutableStateOf(false)}
 var add by remember{mutableStateOf(false)};var overflow by remember{mutableStateOf(false)}
 var dashboardUrlDialog by remember{mutableStateOf(false)};var dashboardRevision by remember{mutableIntStateOf(0)}
 var search by remember{mutableStateOf(false)};var query by remember{mutableStateOf("")}
 var import by remember{mutableStateOf(false)};var themePicker by remember{mutableStateOf(initialPane=="Темы")}
 var message by remember{mutableStateOf("")};var busy by remember{mutableStateOf(false)}
 var deleteProfiles by remember{mutableStateOf<List<ProxyEntity>?>(null)}
 var testJob by remember{mutableStateOf<Job?>(null)}
 var testGeneration by remember{mutableIntStateOf(0)}
 val palettes=remember{JSONArray(AndroidPreferences.javaClass.getResourceAsStream("/android/palettes.json")!!.reader().readText())}
 val palette=remember(themeRevision){(0 until palettes.length()).map{palettes.getJSONArray(it)}.first{it.getString(0)==androidThemeNames[DataStore.appTheme.coerceIn(0,21)]}}
 fun colorAt(index:Int)=Color(palette.getString(index).removePrefix("#").toLong(16)or 0xFF000000L)
 val primary=colorAt(2);val systemDark=isSystemInDarkTheme()
 val dark=when(DataStore.nightTheme){1->true;2->false;3->runCatching{File("/sys/firmware/acpi/platform_profile").readText().trim()=="low-power"}.getOrDefault(false);else->systemDark}
 val colors=if(dark)darkColors(primary=primary,primaryVariant=colorAt(3),secondary=colorAt(4),secondaryVariant=colorAt(4),background=Color(0xFF303030),surface=Color(0xFF424242),onPrimary=Color.White)else lightColors(primary=primary,primaryVariant=colorAt(3),secondary=colorAt(4),secondaryVariant=colorAt(4),background=Color(0xFFFAFAFA),surface=Color.White,onPrimary=Color.White)
 fun refresh(){revision++}
 LaunchedEffect(updates){refresh()}
 fun action(block:suspend()->Unit){scope.launch{busy=true;try{withContext(Dispatchers.IO){block()};refresh()}catch(t:Throwable){if(t !is CancellationException)message=t.message?:"Ошибка"}finally{busy=false}}}
 fun test(list:List<ProxyEntity>,tcp:Boolean=false){
  testJob?.cancel();val generation=++testGeneration;testJob=scope.launch{busy=true;try{
   val semaphore=kotlinx.coroutines.sync.Semaphore(DataStore.connectionTestConcurrent.coerceIn(1,16))
   coroutineScope{list.map{entity->launch{semaphore.acquire();try{
    try{
     entity.ping=if(tcp)withContext(Dispatchers.IO){
      val bean=entity.requireBean();val began=System.nanoTime()
      java.net.Socket().use{it.connect(java.net.InetSocketAddress(bean.serverAddress,bean.serverPort),3000)}
      ((System.nanoTime()-began)/1000000).toInt().coerceAtLeast(1)
     }else LinuxRuntime.urlTest(entity)
     entity.status=1;entity.error=null
    }catch(t:Throwable){if(t is CancellationException)throw t;entity.ping=-1;entity.status=if(tcp)2 else 3;entity.error=if(tcp)AndroidTestPresentation.profileError(t.message)else t.message}
    withContext(Dispatchers.IO){SagerDatabase.proxyDao.updateProxy(entity)};refresh()
   }finally{semaphore.release()}}}.joinAll()}
  }finally{if(generation==testGeneration){busy=false;testJob=null;refresh()}}}
 }
 fun create(id:String){
  val names=mapOf("action_new_tm_split" to "SplitYTM","action_new_socks" to "SOCKS","action_new_http" to "HTTP","action_new_ss" to "Shadowsocks","action_new_vmess" to "VMess","action_new_vless" to "VLESS","action_new_trojan" to "Trojan","action_new_hysteria" to "Hysteria 2","action_new_tuic" to "TUIC","action_new_ssh" to "SSH","action_new_wg" to "WireGuard","action_new_anytls" to "AnyTLS","action_new_shadowtls" to "ShadowTLS","action_new_naive" to "Naïve","action_new_mieru" to "Mieru","action_new_trojan_go" to "Trojan-Go","action_new_config" to "Полная конфигурация sing-box","action_new_chain" to "Цепочка")
  val factory=protocolChoices().find{it.first==names[id]}?.second
  if(factory!=null)editor=ProxyEntity(groupId=groupId).putBean(factory().applyDefaultValues())else message="Неизвестный редактор: $id"
 }
 fun importInput(text:String){
  try{val subscription=LinuxSharing.subscription(text);if(subscription!=null)groupEditor=subscription else action{LinuxProfiles.importText(text,groupId)}}catch(t:Throwable){message=t.message?:"Не удалось импортировать данные"}
 }
 fun groupAction(group:ProxyGroup,id:String){try{when(id){
  "action_universal_clipboard"->{require(group.subscription!=null){"Группа не является подпиской"};copyText(group.toUniversalLink())}
  "action_universal_qr"->{require(group.subscription!=null){"Группа не является подпиской"};qr=group.toUniversalLink() to group.displayName()}
  "action_export_clipboard"->copyText(LinuxSharing.exportGroup(group.id))
  "action_export_file"->chooseFile(true,"группа.txt")?.let{LinuxBackup.write(it.toPath(),LinuxSharing.exportGroup(group.id))}
  "action_clear"->deleteProfiles=SagerDatabase.proxyDao.getByGroup(group.id)
  else->message="Неизвестное действие группы"
 }}catch(t:Throwable){message=t.message?:"Ошибка экспорта группы"}}
 fun menuAction(id:String){when(id){
  "action_scan_qr_code"->{scanner=true}
  "action_import_clipboard"->importInput(pasteText())
  "action_import_file"->{chooseFile(false)?.let{f->if(f.length()>16*1024*1024)message="Файл слишком велик"else importInput(f.readText())}}
  "action_update_subscription"->groups.find{it.id==groupId}?.let{g->action{LinuxProfiles.update(g)}}
  "action_connection_url_test"->test(profiles)
  "action_connection_tcp_ping"->test(profiles,tcp=true)
  "action_connection_test_delete_unavailable"->{deleteProfiles=profiles.filter{it.status>1}}
  "action_remove_duplicate"->{deleteProfiles=LinuxSharing.duplicateIds(profiles)}
  "action_order_origin","action_order_by_name","action_order_by_delay"->action{groups.find{it.id==groupId}?.let{g->g.order=when(id){"action_order_by_name"->1;"action_order_by_delay"->2;else->0};SagerDatabase.groupDao.updateGroup(g)}}
  "action_clear_traffic_statistics"->action{profiles.forEach{it.tx=0;it.rx=0;SagerDatabase.proxyDao.updateProxy(it)}}
  "action_connection_test_clear_results"->action{profiles.forEach{it.ping=0;it.status=0;it.error=null;SagerDatabase.proxyDao.updateProxy(it)}}
  else->if(id.startsWith("action_new_"))create(id)else message="Неизвестное действие: $id"
 }}
 LaunchedEffect(status.error){if(status.error.isNotBlank())message=status.error}
 var rateTx by remember{mutableLongStateOf(0)};var rateRx by remember{mutableLongStateOf(0)}
 LaunchedEffect(status.profile,status.phase){var tx=status.tx;var rx=status.rx;var stamp=System.nanoTime();while(status.phase=="Подключено"){delay(1000);val now=LinuxRuntime.status.value;val end=System.nanoTime();val seconds=(end-stamp)/1e9;rateTx=((now.tx-tx).coerceAtLeast(0)/seconds).toLong();rateRx=((now.rx-rx).coerceAtLeast(0)/seconds).toLong();tx=now.tx;rx=now.rx;stamp=end}}
 val editing=editor!=null||ruleEditor!=null
 val focus=remember{FocusRequester()};LaunchedEffect(Unit){focus.requestFocus()}
 MaterialTheme(colors=colors,typography=androidTypography()) {
 Surface(Modifier.fillMaxSize(),color=colors.background){
  Box(Modifier.fillMaxSize().focusRequester(focus).onPreviewKeyEvent{e->if(e.type==KeyEventType.KeyDown&&e.key==Key.Escape){when{add||overflow->false;drawer->{drawer=false;true};ruleEditor!=null->{ruleEditor=null;true};editor!=null->{editor=null;true};search->{search=false;query="";true};pane!="nav_configuration"->{pane="nav_configuration";true};else->false}}else false}.focusable()){
   if(ruleEditor!=null)ruleEditor?.let{rule->AndroidRuleEditor(rule,{ruleEditor=null},{ruleEditor=null;refresh()})}
   else if(editing) editor?.let{entity->AndroidProfileEditor(entity,{editor=null},{editor=null;refresh()})}
   else Column(Modifier.fillMaxSize()){
    TopAppBar(title={if(search)TextField(query,{query=it},placeholder={Text(string("search"))},singleLine=true,colors=TextFieldDefaults.textFieldColors(textColor=Color.White,backgroundColor=Color.Transparent))else Text(if(pane=="nav_configuration")string("app_name")else if(pane=="stun")string("stun_test")else if(pane=="assets")string("route_assets")else drawerItems.find{it.id==pane}?.title.orEmpty(),fontSize=20.sp,fontWeight=FontWeight.Medium)},
     navigationIcon={AndroidAction(if(pane in listOf("stun","assets"))"baseline_arrow_back_24"else"ic_navigation_menu",string("app_name")){if(pane=="stun")pane="nav_tools"else if(pane=="assets")pane="nav_route"else drawer=true}},backgroundColor=primary,contentColor=Color.White,elevation=4.dp,
     actions={when(pane){
      "nav_traffic"->{Box{AndroidAction("ic_baseline_more_vert_24",string("more")){overflow=true};AndroidPopup(AndroidMenus.load("yacd_menu"),overflow,{overflow=false}){id->if(id=="action_set_url")dashboardUrlDialog=true else pane="nav_configuration"}}}
      "nav_configuration"->{
       AndroidAction("ic_baseline_manage_search_24",string("search")){search=!search;if(!search)query=""}
       Box{AndroidAction("ic_action_note_add",string("add_profile")){add=true};AndroidPopup(AndroidMenus.load("add_profile_menu").find{it.id=="action_add"}?.children.orEmpty(),add,{add=false},::menuAction)}
       Box{AndroidAction("ic_baseline_more_vert_24",string("more")){overflow=true};AndroidPopup(AndroidMenus.load("add_profile_menu").find{it.id=="action_misc"}?.children.orEmpty(),overflow,{overflow=false},::menuAction)}
      }
      "nav_logcat"->{AndroidAction("ic_baseline_refresh_24",string("group_update")){refresh()};AndroidAction("baseline_send_24",string("logcat")){copyText(LinuxRuntime.logs.value)};AndroidAction("baseline_delete_sweep_24",string("clear_logcat")){LinuxRuntime.logs.value=""}}
      "nav_route"->{AndroidAction("ic_baseline_add_road_24",string("route_add")){ruleEditor=RuleEntity(enabled=true)};Box{AndroidAction("ic_baseline_more_vert_24",string("more")){overflow=true};AndroidPopup(AndroidMenus.load("add_route_menu").filter{it.id!="action_new_route"},overflow,{overflow=false}){id->if(id=="action_manage_assets")pane="assets"else if(id=="action_reset_route")resetRoutes=true}}}
      "nav_group"->{AndroidAction("ic_baseline_update_24",string("update_all_subscription")){action{groups.filter{it.subscription!=null}.forEach{LinuxProfiles.update(it)}}};AndroidAction("ic_av_playlist_add",string("group_create")){groupEditor=ProxyGroup(userOrder=groups.size.toLong())}}
      else->Unit
     }})
    if(busy)LinearProgressIndicator(Modifier.fillMaxWidth().height(2.dp),color=primary)
    when(pane){
     "nav_configuration"->Column(Modifier.weight(1f)){
      if(groups.any{!it.ungrouped})ScrollableTabRow(groups.indexOfFirst{it.id==groupId}.coerceAtLeast(0),edgePadding=0.dp,backgroundColor=primary,contentColor=Color.White){groups.forEach{g->Tab(g.id==groupId,{groupId=g.id;DataStore.selectedGroup=g.id},text={Text(g.displayName().uppercase(),fontSize=14.sp)})}}
      LazyColumn(Modifier.fillMaxSize().padding(horizontal=4.dp),contentPadding=PaddingValues(top=4.dp,bottom=48.dp)){
       items(profiles.filter{query.isBlank()||it.displayName().contains(query,true)},key={it.id}){entity->AndroidProfileCard(entity,selected==entity.id,modifier=Modifier.animateItem(fadeInSpec=tween(120),placementSpec=tween(250),fadeOutSpec=tween(120)),select={
        if(selected!=entity.id){selected=entity.id;DataStore.selectedProxy=entity.id;if(status.phase=="Подключено")LinuxRuntime.switchProfile(entity)}
       },edit={editor=entity.copy().putBean(entity.requireBean().clone())},share={id->
        try{when(id){
         "action_standard_clipboard"->copyText(entity.toStdLink())
         "action_universal_clipboard"->copyText(entity.requireBean().toUniversalLink())
         "action_standard_qr"->{require(entity.haveLink()){ "Цепочку экспортируйте вместе с группой" };qr=entity.toStdLink() to entity.displayName()}
         "action_universal_qr"->{require(entity.haveLink()){ "Цепочку экспортируйте вместе с группой" };qr=entity.requireBean().toUniversalLink() to entity.displayName()}
         "action_config_export_clipboard"->copyText(LinuxSharing.config(entity).first)
         "action_config_export_file"->{val exported=LinuxSharing.config(entity);chooseFile(true,exported.second)?.let{LinuxBackup.write(it.toPath(),exported.first)}}
         else->message="Неизвестное действие профиля"
        }}catch(t:Throwable){message=t.message?:"Не удалось экспортировать профиль"}
       })}
      }
     }
     "nav_settings"->PreferenceScreen("global_preferences",DataStore.configurationStore,Modifier.weight(1f)){themeRevision++;refresh()}
     "nav_group"->LazyColumn(Modifier.weight(1f).fillMaxWidth().padding(horizontal=4.dp),contentPadding=PaddingValues(top=4.dp,bottom=48.dp)){
      items(groups,key={it.id}){g->Card(Modifier.padding(4.dp).fillMaxWidth(),shape=RoundedCornerShape(4.dp),elevation=2.dp){Column{
       Row(verticalAlignment=Alignment.CenterVertically){Text(g.displayName(),Modifier.weight(1f).padding(start=16.dp),fontWeight=FontWeight.Bold,fontSize=16.sp);AndroidAction("ic_image_edit",string("edit")){groupEditor=g.copy()};Box{var groupMenu by remember{mutableStateOf(false)};AndroidAction("ic_baseline_more_vert_24",string("more")){groupMenu=true};AndroidPopup(AndroidMenus.load("group_action_menu").filter{it.id!="action_share_subscription"||g.subscription!=null},groupMenu,{groupMenu=false}){id->groupAction(g,id)}}}
       Row(Modifier.padding(start=16.dp,end=8.dp,bottom=8.dp),verticalAlignment=Alignment.CenterVertically){Text(if(g.subscription!=null)string("subscription")else string("group_basic"),Modifier.weight(1f),fontSize=14.sp);if(g.subscription!=null)TextButton(onClick={action{LinuxProfiles.update(g)}}){Text(string("group_update"))}}
      }}}
     }
     "nav_route"->AndroidRulesList(Modifier.weight(1f),revision,{refresh()}){ruleEditor=it.copy()}
     "nav_logcat"->{val log by LinuxRuntime.logs.collectAsState();SelectionContainerCompat(log,Modifier.weight(1f).fillMaxWidth())}
     "nav_tools"->AndroidToolsScreen(Modifier.weight(1f),{pane="stun"},{revision++;themeRevision++;groupId=DataStore.currentGroupId()},if(initialPane=="Резервная копия")1 else 0)
     "stun"->AndroidStunScreen(Modifier.weight(1f))
     "assets"->AndroidAssetsScreen(Modifier.weight(1f))
     "nav_about"->AndroidAboutScreen(Modifier.weight(1f))
     "nav_traffic"->key(dashboardRevision,status.profile,status.phase){AndroidDashboardView(LinuxRuntime.dashboardURL(DataStore.yacdURL),Modifier.weight(1f).fillMaxWidth())}
     else->Spacer(Modifier.weight(1f))
    }
    if(pane=="nav_configuration"||DataStore.showBottomBar){
     AndroidConnectionBar(status.phase,rateTx,rateRx,currentTest,DataStore.connectionTestURL,colorAt(6),primary,colorAt(5),
      test={LinuxRuntime.testCurrentConnection()},toggle={
       if(status.phase!="Отключено")LinuxRuntime.disconnect()else profiles.find{it.id==selected}?.let{LinuxRuntime.connect(it)}?:run{message="Выберите профиль"}
      })
    }

   }
   AnimatedVisibility(drawer&&!editing,enter=fadeIn(tween(256)),exit=fadeOut(tween(200))){Box(Modifier.fillMaxSize().background(Color.Black.copy(alpha=.6f)).clickable{drawer=false})}
   AnimatedVisibility(drawer&&!editing,enter=slideInHorizontally(tween(256,easing=FastOutSlowInEasing)){-it},exit=slideOutHorizontally(tween(200)){ -it }){
    Surface(Modifier.width(320.dp).fillMaxHeight(),elevation=16.dp){LazyColumn(contentPadding=PaddingValues(vertical=8.dp)){
     drawerItems.forEachIndexed{i,item->
      if(i>0&&item.group!=drawerItems[i-1].group)item{Divider(Modifier.padding(vertical=8.dp))}
      item{Row(Modifier.fillMaxWidth().height(48.dp).background(if(pane==item.id)primary.copy(alpha=.12f)else Color.Transparent).clickable{if(item.id in listOf("nav_faq","nav_tuiguang"))java.awt.Desktop.getDesktop().browse(java.net.URI(if(item.id=="nav_faq")"https://matsuridayo.github.io/"else"https://neko-box.pages.dev/喵"))else pane=item.id;drawer=false;search=false},verticalAlignment=Alignment.CenterVertically){AndroidIcon(item.icon,Modifier.padding(start=16.dp).size(24.dp),tint=if(pane==item.id)primary else MaterialTheme.colors.onSurface.copy(alpha=.6f));Text(item.title,Modifier.padding(start=32.dp),fontSize=14.sp,fontWeight=FontWeight.Medium,color=if(pane==item.id)primary else MaterialTheme.colors.onSurface)}}
     }
    }}
   }
   if(message.isNotBlank())Snackbar(Modifier.align(Alignment.BottomCenter).padding(8.dp),action={TextButton(onClick={message=""}){Text("ОК")}}){Text(message)}
  }
 }
 if(scanner)AndroidQrScanner({scanner=false}){text->scanner=false;importInput(text)}
 qr?.let{(text,title)->AndroidQrDialog(text,title){qr=null}}
 if(resetRoutes)AlertDialog(onDismissRequest={resetRoutes=false},title={Text(string("confirm"))},text={Text("Заменить текущие маршруты стандартными отключёнными шаблонами? Работающее соединение не перезапускается.")},confirmButton={TextButton(onClick={resetRoutes=false;action{LinuxRules.reset()}}){Text(string("yes"))}},dismissButton={TextButton(onClick={resetRoutes=false}){Text(string("no"))}})
 if(themePicker)AndroidColorDialog(palettes,{themePicker=false}){DataStore.appTheme=it;themeRevision++;themePicker=false}
 deleteProfiles?.let{toDelete->AlertDialog(onDismissRequest={deleteProfiles=null},title={Text(string("confirm"))},text={Text(string("delete_confirm_prompt")+"\n"+toDelete.take(20).joinToString("\n"){it.displayName()})},confirmButton={TextButton(onClick={deleteProfiles=null;action{require(toDelete.none{it.id==status.profile}){"Сначала отключите удаляемый профиль"};SagerDatabase.proxyDao.deleteProxy(toDelete)}}){Text(string("yes"))}},dismissButton={TextButton(onClick={deleteProfiles=null}){Text(string("no"))}})}
 if(import)TextDialog(string("action_import_file"),pasteText(),"",{import=false}){text->import=false;importInput(text)}
 groupEditor?.let{g->AndroidGroupDialog(g,{groupEditor=null}){groupEditor=null;groupId=DataStore.currentGroupId();refresh()}}
 if(status.phase=="Подключение…"||testJob?.isActive==true)Dialog(onDismissRequest={}){Surface(shape=RoundedCornerShape(4.dp),elevation=24.dp){Column(Modifier.widthIn(min=260.dp),horizontalAlignment=Alignment.CenterHorizontally){FirefoxLoading(AndroidTestPresentation.preparing,vertical=true);TextButton(onClick={if(status.phase=="Подключение…")LinuxRuntime.disconnect()else testJob?.cancel()},modifier=Modifier.align(Alignment.End).padding(end=8.dp,bottom=8.dp)){Text(string("cancel"))}}}}
 if(dashboardUrlDialog)TextDialog(string("set_panel_url"),DataStore.yacdURL,"URL",{dashboardUrlDialog=false}){DataStore.yacdURL=it;dashboardUrlDialog=false;dashboardRevision++}
 }
}

@Composable fun AndroidProfileCard(entity:ProxyEntity,selected:Boolean,select:()->Unit,edit:()->Unit,share:(String)->Unit,modifier:Modifier=Modifier){
 var menu by remember{mutableStateOf(false)}
 Card(modifier.padding(4.dp).fillMaxWidth().animateContentSize().clickable(onClick=select),shape=RoundedCornerShape(4.dp),elevation=2.dp){Row(Modifier.height(IntrinsicSize.Min)){
  Box(Modifier.width(4.dp).fillMaxHeight().background(if(selected)MaterialTheme.colors.primary else Color.Transparent))
  Column(Modifier.weight(1f)){
   Row(verticalAlignment=Alignment.CenterVertically){Text(entity.displayName(),Modifier.weight(1f).padding(start=12.dp,top=8.dp,bottom=8.dp),fontSize=14.sp,fontWeight=FontWeight.Bold,maxLines=2,overflow=TextOverflow.Ellipsis);AndroidAction("ic_image_edit",string("edit"),edit);Box{AndroidAction("ic_social_share",string("share")){menu=true};AndroidPopup(AndroidMenus.load("profile_share_menu"),menu,{menu=false},share)}}
   val traffic=entity.rx+entity.tx>0;val address=if(DataStore.alwaysShowAddress&&entity.requireBean().name.isNotBlank())entity.displayAddress()else ""
   if((traffic&&entity.status>0)||address.isNotBlank())Row(Modifier.padding(start=12.dp,end=8.dp,bottom=4.dp)){
    Text(if(traffic&&address.length>=30)address.take(27)+"..."else address,Modifier.weight(1f),fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))
    if(traffic&&entity.status>0)Text(app.getString("traffic",bytes(entity.tx),bytes(entity.rx)),fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))
   }
   Row(Modifier.fillMaxWidth().padding(start=12.dp,end=8.dp,bottom=12.dp)){
    Text(entity.displayType(),Modifier.weight(1f),fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))
    if(entity.status==1)Text(app.getString("available",entity.ping),fontSize=14.sp,color=Color(0xFF4CAF50))else if(entity.status>1)Text(AndroidTestPresentation.profileError(entity.error),fontSize=14.sp,color=Color(0xFFF44336),maxLines=2,softWrap=true,textAlign=androidx.compose.ui.text.style.TextAlign.End,modifier=Modifier.weight(1f))else if(traffic)Text(app.getString("traffic",bytes(entity.tx),bytes(entity.rx)),fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))
   }
  }
 }}
}

@Composable fun AndroidProfileEditor(entity:ProxyEntity,dismiss:()->Unit,save:()->Unit){
 val editor=remember(entity){LinuxProfiles.editorLoad(entity)};var error by remember{mutableStateOf("")}
 Column(Modifier.fillMaxSize()){
  TopAppBar(title={Text(entity.displayType())},backgroundColor=MaterialTheme.colors.primary,contentColor=Color.White,navigationIcon={AndroidAction("baseline_arrow_back_24",string("cancel"),dismiss)},actions={
   if(entity.id!=0L)AndroidAction("ic_action_delete",string("delete")){SagerDatabase.proxyDao.deleteById(entity.id);save()}
   AndroidAction("ic_action_done",string("apply")){try{LinuxProfiles.editorSave(entity,editor);save()}catch(t:Throwable){error=t.message?:"Ошибка"}}
  })
  if(entity.type==ProxyEntity.TYPE_CHAIN)AndroidChainFields(entity,Modifier.weight(1f))else key(entity){PreferenceScreen(AndroidPreferences.editorScreen(entity),DataStore.profileCacheStore,Modifier.weight(1f),entity=entity)}
  if(error.isNotBlank())Text(error,Modifier.padding(16.dp),color=MaterialTheme.colors.error)
 }
}
@Composable fun AndroidGroupDialog(group:ProxyGroup,dismiss:()->Unit,save:()->Unit){
 remember(group){group.subscription=group.subscription?.let{moe.matsuri.nb4a.utils.JavaUtil.gson.fromJson(moe.matsuri.nb4a.utils.JavaUtil.gson.toJson(it),SubscriptionBean::class.java)};true}
 var name by remember{mutableStateOf(group.name.orEmpty())};var link by remember{mutableStateOf(group.subscription?.link.orEmpty())};var error by remember{mutableStateOf("")}
 var advanced by remember{mutableStateOf(false)};var remove by remember{mutableStateOf(false)}
 AlertDialog(onDismissRequest=dismiss,title={Text(string(if(group.id==0L)"group_create"else"group_edit"))},text={Column(Modifier.fillMaxWidth()){
  if(advanced)PreferenceScreen("group_preferences",DataStore.profileCacheStore,Modifier.fillMaxWidth().heightIn(min=240.dp,max=430.dp))
  else{
   TextField(name,{name=it},label={Text(string("group_name"))},modifier=Modifier.fillMaxWidth())
   Spacer(Modifier.height(8.dp));TextField(link,{link=it},label={Text(string("group_subscription_link"))},modifier=Modifier.fillMaxWidth())
   TextButton(onClick={group.name=name;group.type=if(link.isBlank())0 else 1;group.subscription=(group.subscription?:SubscriptionBean().applyDefaultValues()).apply{this.link=link};group.initLinux();advanced=true}){Text("Дополнительно")}
  }
  if(group.id>0&&!group.ungrouped)TextButton(onClick={remove=true}){Text(string("delete"),color=MaterialTheme.colors.error)}
  if(error.isNotBlank())Text(error,color=MaterialTheme.colors.error)
 }},confirmButton={TextButton(onClick={try{
  if(advanced)group.serializeLinux()else{group.name=name.trim();group.type=if(link.isBlank())0 else 1;group.subscription=if(link.isBlank())null else(group.subscription?:SubscriptionBean().applyDefaultValues()).apply{this.link=link.trim()}}
  require(!group.name.isNullOrBlank()||group.ungrouped){"Укажите имя группы"}
  if(group.type==1){val text=group.subscription?.link.orEmpty();val inline=text.trimStart().startsWith("{")||text.contains("proxies:");if(!inline){val uri=java.net.URI(text);require(uri.scheme in listOf("http","https")&&uri.host!=null){"Ссылка подписки должна использовать HTTP или HTTPS"}};require(group.subscription!!.autoUpdateDelay>0){"Интервал обновления должен быть положительным"}}
  if(group.id==0L)group.id=SagerDatabase.groupDao.createGroup(group)else SagerDatabase.groupDao.updateGroup(group);save()
 }catch(t:Throwable){error=t.message?:"Ошибка"}}){Text(string("apply"))}},dismissButton={TextButton(onClick=dismiss){Text(string("cancel"))}})
 if(remove)AlertDialog(onDismissRequest={remove=false},title={Text(string("confirm"))},text={Text("Удалить группу и все её профили?")},confirmButton={TextButton(onClick={try{
  val profiles=SagerDatabase.proxyDao.getByGroup(group.id);require(profiles.none{it.id==LinuxRuntime.status.value.profile}){"Сначала отключите профиль этой группы"}
  SagerDatabase.runInTransaction{SagerDatabase.proxyDao.deleteByGroup(group.id);SagerDatabase.groupDao.deleteById(group.id)}
  if(DataStore.selectedGroup==group.id)DataStore.selectedGroup=SagerDatabase.groupDao.allGroups().firstOrNull()?.id?:0
  remove=false;save()
 }catch(t:Throwable){remove=false;error=t.message?:"Не удалось удалить группу"}}){Text(string("yes"))}},dismissButton={TextButton(onClick={remove=false}){Text(string("no"))}})
}

fun chooseFile(save:Boolean,suggested:String?=null):File? {
 val dialog=java.awt.FileDialog(null as java.awt.Frame?,if(save)"Сохранить"else"Открыть",if(save)java.awt.FileDialog.SAVE else java.awt.FileDialog.LOAD)
 if(suggested!=null)dialog.file=suggested.replace('/','_').replace('\\','_').replace('\u0000','_')
 try{dialog.isVisible=true;return dialog.file?.let{File(dialog.directory,it)}}finally{dialog.dispose()}
}
@Composable fun AndroidColorDialog(palettes:JSONArray,dismiss:()->Unit,selected:(Int)->Unit){
 AlertDialog(onDismissRequest=dismiss,title={Text(string("theme"))},text={Column(horizontalAlignment=Alignment.CenterHorizontally){
  for(row in (1..21).chunked(4))Row{for(id in row){val p=(0 until palettes.length()).map{palettes.getJSONArray(it)}.first{it.getString(0)==androidThemeNames[id]};Box(Modifier.size(64.dp).clickable{selected(id)},contentAlignment=Alignment.Center){AndroidIcon("ic_baseline_fiber_manual_record_24",Modifier.size(64.dp),Color(p.getString(2).removePrefix("#").toLong(16)or 0xFF000000L))}}}
 }},confirmButton={},dismissButton={TextButton(onClick=dismiss){Text(string("cancel"))}})
}

@Composable fun AndroidRulesList(modifier:Modifier,revision:Int,refresh:()->Unit,edit:(RuleEntity)->Unit){
 val rules=remember(revision){SagerDatabase.rulesDao.allRules()}
 LazyColumn(modifier.fillMaxWidth().padding(horizontal=4.dp),contentPadding=PaddingValues(top=4.dp,bottom=48.dp)){
  if(rules.isEmpty())item{Card(Modifier.padding(4.dp).fillMaxWidth().clickable{java.awt.Desktop.getDesktop().browse(java.net.URI("https://matsuridayo.github.io/nb4a-route/"))},elevation=2.dp,shape=RoundedCornerShape(4.dp)){Text(string("route_warn"),Modifier.padding(16.dp),fontWeight=FontWeight.Bold,fontSize=14.sp)}}
  items(rules,key={it.id}){rule->
   fun toggle(enabled:Boolean){rule.enabled=enabled;SagerDatabase.rulesDao.updateRule(rule);refresh()}
   Card(Modifier.padding(4.dp).fillMaxWidth().clickable{toggle(!rule.enabled)},elevation=2.dp,shape=RoundedCornerShape(4.dp)){Column{
    Row(verticalAlignment=Alignment.CenterVertically){Text(rule.displayName(),Modifier.weight(1f).padding(start=12.dp,top=8.dp,bottom=8.dp),fontSize=14.sp,fontWeight=FontWeight.Bold);AndroidAction("ic_image_edit",string("edit")){edit(rule)}}
    Row(Modifier.padding(start=12.dp,end=8.dp),verticalAlignment=Alignment.Bottom){Column(Modifier.weight(1f)){
     Text(rule.mkSummary(),fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f));Text(rule.displayOutbound(),Modifier.padding(top=4.dp,bottom=12.dp),fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))
    };Switch(rule.enabled,::toggle,colors=SwitchDefaults.colors(checkedThumbColor=MaterialTheme.colors.primary))}
   }}
  }
 }
}
@Composable fun AndroidRuleEditor(rule:RuleEntity,dismiss:()->Unit,save:()->Unit){
 remember(rule){DataStore.editingId=rule.id;rule.initLinux();true};var error by remember{mutableStateOf("")}
 Column(Modifier.fillMaxSize()){
  TopAppBar(title={Text(string("cag_route"))},backgroundColor=MaterialTheme.colors.primary,contentColor=Color.White,navigationIcon={AndroidAction("baseline_arrow_back_24",string("cancel"),dismiss)},actions={
   if(rule.id!=0L)AndroidAction("ic_action_delete",string("delete")){SagerDatabase.rulesDao.deleteById(rule.id);save()}
   AndroidAction("ic_action_done",string("apply")){try{rule.serializeLinux();require(rule.mkSummary().isNotBlank()){string("empty_route")};if(rule.id==0L){rule.userOrder=SagerDatabase.rulesDao.nextOrder()?:0;SagerDatabase.rulesDao.createRule(rule)}else SagerDatabase.rulesDao.updateRule(rule);save()}catch(t:Throwable){error=t.message?:"Ошибка"}}
  })
  PreferenceScreen("route_preferences",DataStore.profileCacheStore,Modifier.weight(1f))
  if(error.isNotBlank())Text(error,Modifier.padding(16.dp),color=MaterialTheme.colors.error)
 }
}
