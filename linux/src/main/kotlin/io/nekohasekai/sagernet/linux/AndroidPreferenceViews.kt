@file:OptIn(androidx.compose.material.ExperimentalMaterialApi::class)
package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.*
import androidx.compose.material.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.*
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.nekohasekai.sagernet.database.*
import io.nekohasekai.sagernet.ktx.app
import org.json.JSONArray

/** Видимость соответствует обработчикам исходных SettingsActivity, а не перечню всех полей. */
fun androidVisiblePreferences(name:String,store:PreferenceStore,entity:ProxyEntity?=null):List<PreferenceSpec>{
 val specs=AndroidPreferences.screen(name)
 fun value(key:String,default:String="")=store.getString(key)?:specs.find{it.key==key}?.defaultValue?.ifBlank{default}?:default
 val hidden=mutableSetOf<String>()
 if(name=="standard_v2ray_preferences"){
  val http=entity?.type==ProxyEntity.TYPE_HTTP;val vmess=entity?.type==ProxyEntity.TYPE_VMESS;val vless=entity?.requireBean()?.let{it is io.nekohasekai.sagernet.fmt.v2ray.VMessBean && it.isVLESS}?:false
  if(http)hidden+=listOf("type","uuid")
  if(!vmess&&!vless)hidden+="packetEncoding"
  if(!vmess||vless)hidden+="alterId"
  if(!vmess&&!vless)hidden+="encryption"
  if(!http)hidden+=listOf("username","password")
  val transport=value("type","tcp")
  if(transport !in listOf("http","ws","httpupgrade"))hidden+="host"
  if(transport !in listOf("http","ws","grpc","httpupgrade"))hidden+="path"
  if(transport!="ws")hidden+="serverWsCategory"
  if(!value("security").contains("tls"))hidden+=listOf("serverSecurityCategory","serverTlsCamouflageCategory","serverECHCategory")
 }
 if(name=="group_preferences"&&value("groupType","0")=="0")hidden+=listOf("groupSubscription","subscriptionUpdate")
 if(name=="socks_preferences"&&value("serverProtocol","2")!="2")hidden+="serverPassword"
 return specs.filter{it.key !in hidden&&it.parents.none{p->p in hidden}}.map{p->
  if(name!="standard_v2ray_preferences")p else when{
   p.key=="uuid"&&entity?.type==ProxyEntity.TYPE_TROJAN->p.copy(title=app.getString("password"))
   p.key=="encryption"&&entity?.requireBean()?.let{it is io.nekohasekai.sagernet.fmt.v2ray.VMessBean&&it.isVLESS}==true->p.copy(title=app.getString("xtls_flow"),icon="ic_baseline_stream_24",entries=AndroidPreferences.array("@array/xtls_flow_value"),values=AndroidPreferences.array("@array/xtls_flow_value"))
   p.key=="encryption"->p.copy(entries=AndroidPreferences.array("@array/vmess_encryption_value"),values=AndroidPreferences.array("@array/vmess_encryption_value"))
   p.key in listOf("host","path")->{val t=value("type","tcp");val key=if(p.key=="host")when(t){"ws"->"ws_host";"httpupgrade"->"http_upgrade_host";else->"http_host"}else when(t){"grpc"->"grpc_service_name";"ws"->"ws_path";"httpupgrade"->"http_upgrade_path";else->"http_path"};p.copy(title=app.getString(key))}
   else->p
  }
 }
}
@Composable fun PreferenceScreen(name:String,store:PreferenceStore,modifier:Modifier=Modifier,entity:ProxyEntity?=null,onChange:()->Unit={}){
 var revision by remember(name){mutableIntStateOf(0)}
 val specs=remember(name,revision,entity){androidVisiblePreferences(name,store,entity)}
 LazyColumn(modifier.fillMaxWidth()){
  items(specs){p->if(p.type=="OutboundPreference")OutboundItem(p) else if(p.category)Text(p.title,Modifier.fillMaxWidth().padding(start=72.dp,top=16.dp,end=16.dp,bottom=8.dp),fontSize=14.sp,color=MaterialTheme.colors.primary,fontWeight=FontWeight.Medium)
   else if(p.key.isNotBlank())PreferenceItem(p,store){revision++;onChange()}
  }
 }
}
@Composable fun PreferenceItem(pref:PreferenceSpec,store:PreferenceStore,onChange:()->Unit){
 if(pref.key=="proxyApps"){LinuxProcessPreference(pref,onChange);return}
 var value by remember(pref.key){mutableStateOf(AndroidPreferences.get(store,pref))}
 var dialog by remember{mutableStateOf(false)};var draft by remember{mutableStateOf(value)};var reveal by remember{mutableStateOf(false)}
 val isSwitch=pref.type.contains("Switch")||pref.type.contains("CheckBox")
 val secret=pref.attributes["dialogLayout"]=="@layout/layout_password_dialog"||pref.key.contains("password",true)||pref.key in listOf("uuid","privateKey")
 val index=pref.values.indexOf(value)
 val summary=when{isSwitch->pref.summary;secret&&value.isNotBlank()->"••••••••";index>=0->pref.entries.getOrElse(index){value};pref.attributes["useSimpleSummaryProvider"]=="true"->value.ifBlank{app.getString("not_set")};else->pref.summary}
 val enabled=pref.attributes["enabled"]!="false"&&(pref.attributes["dependency"]?.let{store.getString(it)?.toBoolean()?:false}?:true)
 fun commit(v:String){store.putString(pref.key,v);value=v;onChange()}
 Row(Modifier.fillMaxWidth().heightIn(min=64.dp).clickable(enabled=enabled){if(isSwitch)commit((!value.toBoolean()).toString())else{draft=value;dialog=true}}.padding(start=16.dp,end=16.dp,top=12.dp,bottom=12.dp),verticalAlignment=Alignment.CenterVertically){
  Box(Modifier.width(56.dp),contentAlignment=Alignment.CenterStart){if(pref.icon.isNotBlank())AndroidIcon(pref.icon,tint=LocalContentColor.current.copy(alpha=if(enabled).6f else .25f))}
  Column(Modifier.weight(1f)){
   Text(pref.title,fontSize=16.sp,color=LocalContentColor.current.copy(alpha=if(enabled)1f else .38f))
   if(summary.isNotBlank())Text(summary,fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))
  }
  if(isSwitch)Switch(value.toBoolean(),{commit(it.toString())},enabled=enabled,colors=SwitchDefaults.colors(checkedThumbColor=MaterialTheme.colors.primary,checkedTrackColor=MaterialTheme.colors.primary))
  else if(pref.type=="ColorPickerPreference")AndroidIcon("ic_baseline_fiber_manual_record_24",Modifier.size(48.dp),MaterialTheme.colors.primary)
 }
 if(dialog){
  if(pref.type=="ColorPickerPreference")AndroidColorDialog(remember{JSONArray(AndroidPreferences.javaClass.getResourceAsStream("/android/palettes.json")!!.reader().readText())},{dialog=false}){commit(it.toString());dialog=false}
  else if(pref.entries.isNotEmpty())AlertDialog(onDismissRequest={dialog=false},title={Text(pref.title)},text={Column(Modifier.heightIn(max=440.dp).verticalScroll(rememberScrollState())){pref.entries.forEachIndexed{i,label->Row(Modifier.fillMaxWidth().heightIn(min=48.dp).clickable{commit(pref.values.getOrElse(i){i.toString()});dialog=false},verticalAlignment=Alignment.CenterVertically){RadioButton(value==pref.values.getOrElse(i){i.toString()},null);Text(label,Modifier.padding(start=16.dp),fontSize=16.sp)}}}},confirmButton={},dismissButton={TextButton(onClick={dialog=false}){Text(app.getString("cancel"))}})
  else AlertDialog(onDismissRequest={dialog=false},title={Text(AndroidPreferences.text(pref.attributes["dialogTitle"]?:pref.title))},text={Column{
   val hint=AndroidPreferences.text(pref.attributes["dialogMessage"].orEmpty());if(hint.isNotBlank())Text(hint,Modifier.padding(bottom=16.dp),fontSize=14.sp)
   TextField(draft,{draft=it},modifier=Modifier.fillMaxWidth(),singleLine=secret||pref.key in listOf("serverAddress","serverPort","name","profileName"),visualTransformation=if(secret&&!reveal)PasswordVisualTransformation()else VisualTransformation.None,textStyle=LocalTextStyle.current.copy(fontFamily=if(secret)FontFamily.Monospace else FontFamily.Default),trailingIcon=if(secret){{TextButton(onClick={reveal=!reveal}){AndroidIcon("ic_baseline_visibility_24")}}}else null)
  }},confirmButton={TextButton(onClick={commit(draft);dialog=false}){Text("ОК")}},dismissButton={TextButton(onClick={dialog=false}){Text(app.getString("cancel"))}})
 }
}
