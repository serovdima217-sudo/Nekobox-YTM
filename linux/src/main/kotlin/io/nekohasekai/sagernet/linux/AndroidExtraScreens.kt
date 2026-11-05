@file:OptIn(androidx.compose.material.ExperimentalMaterialApi::class)
package io.nekohasekai.sagernet.linux
import androidx.compose.runtime.*
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.ktx.app
import kotlinx.coroutines.*
import java.nio.file.*
import org.json.JSONObject

private fun s(key:String)=app.getString(key)
@Composable private fun SourceCard(modifier:Modifier=Modifier,content:@Composable ColumnScope.()->Unit){Card(modifier.fillMaxWidth(),shape=RoundedCornerShape(4.dp),elevation=2.dp){Column(Modifier.padding(16.dp),content=content)}}
@Composable fun AndroidToolsScreen(modifier:Modifier,onStun:()->Unit,onChanged:()->Unit,initialTab:Int=0){
 var tab by remember{mutableIntStateOf(initialTab)}
 Column(modifier.fillMaxSize()){
  TabRow(tab,backgroundColor=MaterialTheme.colors.primary,contentColor=androidx.compose.ui.graphics.Color.White){listOf("tools_network","backup").forEachIndexed{i,key->Tab(tab==i,{tab=i},text={Text(s(key).uppercase())})}}
  if(tab==0)Column(Modifier.weight(1f).padding(8.dp)){SourceCard{
   Text(s("stun_test"),fontSize=20.sp,fontWeight=FontWeight.Medium);Text(s("stun_test_summary"),Modifier.padding(top=16.dp),fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))
   Row(Modifier.fillMaxWidth().padding(top=16.dp),horizontalArrangement=Arrangement.End){TextButton(onClick=onStun){Text(s("start").uppercase())}}
  }}else AndroidBackupScreen(Modifier.weight(1f),onChanged)
 }
}
@Composable fun AndroidBackupScreen(modifier:Modifier,onChanged:()->Unit){
 var profiles by remember{mutableStateOf(true)};var rules by remember{mutableStateOf(true)};var settings by remember{mutableStateOf(true)}
 var reset by remember{mutableStateOf(false)};var import by remember{mutableStateOf<JSONObject?>(null)};var message by remember{mutableStateOf("")};var busy by remember{mutableStateOf(false)}
 val scope=rememberCoroutineScope()
 fun run(action:()->Unit){scope.launch{busy=true;try{withContext(Dispatchers.IO){action()};onChanged()}catch(t:Throwable){message=t.message?:"Ошибка"}finally{busy=false}}}
 Column(modifier.padding(16.dp).verticalScroll(rememberScrollState())){
  Button(onClick={reset=true},modifier=Modifier.fillMaxWidth(),enabled=!busy){Text(s("reset_settings").uppercase())}
  listOf("backup_groups_and_configurations" to profiles,"backup_rules" to rules,"backup_settings" to settings).forEachIndexed{i,(key,checked)->Row(Modifier.fillMaxWidth().clickable{when(i){0->profiles=!profiles;1->rules=!rules;else->settings=!settings}},verticalAlignment=Alignment.CenterVertically){Checkbox(checked,{when(i){0->profiles=it;1->rules=it;else->settings=it}});Text(s(key),fontWeight=FontWeight.Medium,fontSize=14.sp)}}
  Button(onClick={chooseFile(true)?.let{file->run{LinuxBackup.write(file.toPath(),LinuxBackup.export(profiles,rules,settings));message=s("action_export_msg")}}},modifier=Modifier.fillMaxWidth(),enabled=!busy){Text(s("action_export").uppercase())}
  Button(onClick={run{copyText(LinuxBackup.export(profiles,rules,settings));message="Резервная копия скопирована в буфер обмена"}},modifier=Modifier.fillMaxWidth(),enabled=!busy){Text(s("share").uppercase())}
  Text(s("backup_summary"),Modifier.padding(top=16.dp),fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))
  Button(onClick={chooseFile(false)?.let{file->run{import=LinuxBackup.parse(file.readText())}}},modifier=Modifier.fillMaxWidth().padding(top=16.dp),enabled=!busy){Text(s("action_import_file").uppercase())}
  if(busy)LinearProgressIndicator(Modifier.fillMaxWidth());if(message.isNotBlank())SelectionContainer{Text(message,Modifier.padding(top=16.dp))}
 }
 if(reset)AlertDialog(onDismissRequest={reset=false},title={Text(s("confirm"))},text={Text(s("reset_settings_message"))},confirmButton={TextButton(onClick={reset=false;run{LinuxBackup.resetSettings();message="Настройки сброшены. Профили и маршруты сохранены."}}){Text(s("yes"))}},dismissButton={TextButton(onClick={reset=false}){Text(s("no"))}})
 import?.let{json->AlertDialog(onDismissRequest={import=null},title={Text(s("backup_import"))},text={Text("Заменить выбранные данные резервной копией? Перед восстановлением автоматически сохраняется текущая копия. Android Parcel не импортируется этим Linux-форматом.")},confirmButton={TextButton(onClick={import=null;run{LinuxBackup.restore(json,profiles,rules,settings);message="Резервная копия восстановлена"}}){Text(s("backup_import"))}},dismissButton={TextButton(onClick={import=null}){Text(s("cancel"))}})}
}
@Composable fun AndroidStunScreen(modifier:Modifier){
 var server by remember{mutableStateOf("stun.voipgate.com:3478")};var result by remember{mutableStateOf("")};var busy by remember{mutableStateOf(false)};val scope=rememberCoroutineScope()
 if(busy)Box(modifier.fillMaxSize(),contentAlignment=Alignment.Center){FirefoxLoading(s("stun_attest_loading"),vertical=true)}
 else Column(modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp)){
  SourceCard{Text(s("nat_stun_server_hint"),fontWeight=FontWeight.Medium,fontSize=14.sp);TextField(server,{server=it},modifier=Modifier.fillMaxWidth(),singleLine=true)}
  SourceCard(Modifier.padding(top=16.dp)){Text(s("nat_result_hint"),fontWeight=FontWeight.Medium,fontSize=14.sp);SelectionContainer{Text(result,Modifier.padding(top=8.dp))}}
  Row(Modifier.fillMaxWidth().padding(top=16.dp),horizontalArrangement=Arrangement.End){TextButton(onClick={scope.launch{busy=true;try{result=withContext(Dispatchers.IO){LinuxStun.test(server)}}catch(t:Throwable){result=t.message?:"Ошибка"}finally{busy=false}}}){Text(s("start").uppercase())}}
 }
}
@Composable fun AndroidAboutScreen(modifier:Modifier){
 fun open(url:String)=java.awt.Desktop.getDesktop().browse(java.net.URI(url))
 Column(modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(8.dp)){
  SourceCard{Text(s("app_name_long"),fontWeight=FontWeight.Medium,fontSize=14.sp);Text(s("app_desc"),Modifier.padding(top=16.dp),fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))}
  SourceCard(Modifier.padding(top=8.dp)){
   ListItem(Modifier.clickable{open("https://github.com/MatsuriDayo/NekoBoxForAndroid/releases")},icon={AndroidIcon("ic_baseline_update_24")},text={Text(s("app_version"))},secondaryText={Text("NekoBox 1.4.2 / Linux x86_64")})
   TextButton(onClick={open("https://github.com/MatsuriDayo/NekoBoxForAndroid/releases/latest")}){Text(s("check_update_release"))}
   TextButton(onClick={open("https://github.com/MatsuriDayo/NekoBoxForAndroid/releases/tag/preview")}){Text(s("check_update_preview"))}
   Text("Ядро sing-box и SplitYTM · JVM 25 · GPL-3.0-or-later",fontSize=14.sp)
  }
  SourceCard(Modifier.padding(top=8.dp)){Text(s("project"),fontWeight=FontWeight.Medium);ListItem(Modifier.clickable{open("https://github.com/MatsuriDayo/NekoBoxForAndroid")},icon={AndroidIcon("ic_baseline_sanitizer_24")},text={Text(s("github"))});ListItem(Modifier.clickable{open("https://t.me/MatsuriDayo")},icon={AndroidIcon("ic_qu_shadowsocks_foreground")},text={Text(s("telegram"))})}
  SourceCard(Modifier.padding(top=8.dp)){Text(s("license"),fontWeight=FontWeight.Medium);val license=remember{AndroidPreferences.javaClass.getResourceAsStream("/android/assets/LICENSE")!!.reader().readText()};SelectionContainer{Text(license+"\nLinux-перенос: GPL-3.0-or-later.\nRoboto: Apache-2.0.\nFirefox loading: MPL-2.0.",Modifier.padding(top=16.dp),fontSize=12.sp)}}
 }
}
@Composable fun AndroidAssetsScreen(modifier:Modifier){
 val directory=remember{LinuxDatabase.directory.resolve("assets").also{Files.createDirectories(it)}}
 var revision by remember{mutableIntStateOf(0)};var message by remember{mutableStateOf("")}
 val files=remember(revision){Files.list(directory).use{it.filter{f->Files.isRegularFile(f)}.sorted().toList()}}
 Column(modifier.fillMaxSize()){
  TextButton(onClick={chooseFile(false)?.let{f->try{require(f.extension in listOf("db","srs")){"Выберите файл .db или .srs"};Files.copy(f.toPath(),directory.resolve(f.name),StandardCopyOption.REPLACE_EXISTING);revision++}catch(t:Throwable){message=t.message?:"Ошибка"}}}){Text(s("action_import_file"))}
  if(message.isNotBlank())Text(message,Modifier.padding(16.dp),color=MaterialTheme.colors.error)
  LazyColumn{items(files,key={it.toString()}){f->Card(Modifier.padding(horizontal=8.dp,vertical=4.dp).fillMaxWidth(),shape=RoundedCornerShape(4.dp),elevation=2.dp){Column(Modifier.padding(16.dp)){Text(f.fileName.toString(),fontSize=18.sp,fontWeight=FontWeight.Bold);Text(bytes(Files.size(f)),Modifier.padding(top=12.dp),fontSize=14.sp)}}}}
 }
}
