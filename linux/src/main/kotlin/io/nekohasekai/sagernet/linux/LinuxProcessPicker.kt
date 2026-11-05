package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.*
import androidx.compose.material.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.nekohasekai.sagernet.database.DataStore
import kotlinx.coroutines.*

@Composable fun LinuxProcessPreference(pref:PreferenceSpec,onChange:()->Unit) {
 var opened by remember{mutableStateOf(false)}
 var enabled by remember{mutableStateOf(DataStore.proxyApps)}
 var count by remember{mutableIntStateOf(LinuxProcessBypass.selected().size)}
 var message by remember{mutableStateOf("")}
 var busy by remember{mutableStateOf(false)}
 val scope=rememberCoroutineScope()
 fun applySaved() {scope.launch {
  busy=true;message="Применение исключений…"
  try { val active=LinuxRuntime.applyProcessBypass();message=if(active)"Исключения применены к новым соединениям"else"Сохранено. Применится при подключении VPN" }
  catch(t:Throwable){message="Не удалось применить: ${t.message?:"неизвестная ошибка"}. Переподключите VPN."}
  finally{busy=false}
 }}
 Row(Modifier.fillMaxWidth().heightIn(min=64.dp).clickable(enabled=!busy){opened=true}.padding(horizontal=16.dp,vertical=12.dp),verticalAlignment=Alignment.CenterVertically){
  Box(Modifier.width(56.dp)){AndroidIcon(pref.icon,tint=LocalContentColor.current.copy(alpha=.6f))}
  Column(Modifier.weight(1f)){
   Text("VPN для процессов",fontSize=16.sp)
   Text(if(busy)message else if(enabled)"Без VPN: $count. Нажмите для выбора процессов"else"Выберите процессы, которым не нужен VPN",fontSize=14.sp,color=LocalContentColor.current.copy(alpha=.6f))
   if(!busy&&message.isNotBlank())Text(message,fontSize=12.sp,color=MaterialTheme.colors.primary)
  }
  Switch(enabled,{value->enabled=value;DataStore.proxyApps=value;onChange();applySaved()},enabled=!busy)
 }
 if(opened)LinuxProcessPicker(onDismiss={opened=false}){paths,useBypass->
  LinuxProcessBypass.save(paths,useBypass);enabled=useBypass;count=LinuxProcessBypass.selected().size
  opened=false;onChange();applySaved()
 }
}

@Composable fun LinuxProcessPicker(onDismiss:()->Unit,onSave:(Set<String>,Boolean)->Unit) {
 var selected by remember{mutableStateOf(LinuxProcessBypass.selected().toSet())}
 var enabled by remember{mutableStateOf(DataStore.proxyApps)}
 var active by remember{mutableStateOf(emptyList<LinuxActiveProcess>())}
 var query by remember{mutableStateOf("")}
 var loading by remember{mutableStateOf(true)}
 var message by remember{mutableStateOf("")}
 var help by remember{mutableStateOf(false)}
 var authorize by remember{mutableStateOf(false)}
 var privileged by remember{mutableStateOf(false)}
 var refresh by remember{mutableIntStateOf(0)}
 LaunchedEffect(refresh){
  loading=true
  try{active=if(privileged)LinuxProcessBypass.privileged()else withContext(Dispatchers.IO){LinuxProcessBypass.active()};message=""}
  catch(t:Throwable){if(t is CancellationException)throw t;message=t.message?:"Не удалось прочитать процессы";privileged=false}
  finally{loading=false}
 }
 AlertDialog(onDismissRequest=onDismiss,modifier=Modifier.widthIn(max=680.dp).fillMaxHeight(.92f),title={Text("Процессы без VPN")},text={
  Column(Modifier.fillMaxWidth().fillMaxHeight()){
   Row(Modifier.fillMaxWidth(),verticalAlignment=Alignment.CenterVertically){
    Text("Включить исключения",Modifier.weight(1f));Switch(enabled,{enabled=it})
   }
   Text("Выбор действует на все PID одного исполняемого файла. Режим VPN/TUN.",fontSize=12.sp,color=LocalContentColor.current.copy(alpha=.65f))
   OutlinedTextField(query,{query=it},Modifier.fillMaxWidth().padding(top=6.dp),label={Text("Имя, путь, PID или UID")},singleLine=true)
   Row(Modifier.fillMaxWidth(),verticalAlignment=Alignment.CenterVertically){
    AndroidAction("ic_baseline_refresh_24","Обновить"){if(!loading)refresh++}
    TextButton(onClick={authorize=true},enabled=!loading){Text("Системные пути…")}
    Spacer(Modifier.weight(1f));AndroidAction("ic_baseline_info_24","Справка"){help=true}
   }
   Row(Modifier.fillMaxWidth(),verticalAlignment=Alignment.CenterVertically){
    Text("Выбрано файлов: ${selected.size} · PID: ${active.sumOf{it.pids.size}}",Modifier.weight(1f),fontSize=12.sp)
    if(selected.isNotEmpty())TextButton(onClick={selected=emptySet()}){Text("Снять")}
   }
   if(message.isNotBlank())Text(message,Modifier.padding(vertical=4.dp),fontSize=12.sp,color=MaterialTheme.colors.error)
   LinuxProcessList(active,selected,query,loading,{path->selected=if(path in selected)selected-path else selected+path;if(selected.isNotEmpty())enabled=true},Modifier.weight(1f))
  }
 },confirmButton={TextButton(onClick={onSave(selected,enabled)},enabled=!loading){Text("Применить")}},dismissButton={TextButton(onClick=onDismiss){Text("Отмена")}})
 if(authorize)AlertDialog(onDismissRequest={authorize=false},title={Text("Доступ к системным путям")},text={Text("Linux скрывает исполняемые файлы некоторых чужих процессов. Откроется запрос прав администратора только для чтения /proc. Работающий VPN не перезапускается и не перенастраивается.")},confirmButton={TextButton(onClick={authorize=false;privileged=true;refresh++}){Text("Прочитать")}},dismissButton={TextButton(onClick={authorize=false}){Text("Отмена")}})
 if(help)AlertDialog(onDismissRequest={help=false},title={Text("Выбор процессов")},text={
  Column(Modifier.verticalScroll(rememberScrollState())){
   Text("Показаны отдельные PID, в том числе чужих пользователей и системные. Для одного файла выбор общий: его текущие и будущие процессы идут напрямую, пока путь не изменится. PID не сохраняются.")
   Text("\nНедоступный путь не скрывает процесс из списка. Для его чтения нажмите «Системные пути…». Процессы UID 0 уже исключены из TUN, а потоки ядра не имеют исполняемого файла.")
   Text("\nПосле обновления приложения или запуска AppImage путь может измениться — выберите процесс заново. Общий системный DNS может оставаться в VPN.")
   Text("\n«Применить» обновляет маршруты без переподключения пары SplitYTM. Открытые соединения могут прерваться.")
  }
 },confirmButton={TextButton(onClick={help=false}){Text("ОК")}})
}

@Composable fun LinuxProcessList(active:List<LinuxActiveProcess>,selected:Set<String>,query:String,loading:Boolean,onToggle:(String)->Unit,modifier:Modifier=Modifier) {
 val entries=remember(active,selected,query){LinuxProcessBypass.entries(active,selected,query)}
 if(loading){Box(modifier.fillMaxWidth(),contentAlignment=Alignment.Center){CircularProgressIndicator()};return}
 if(entries.isEmpty()){Box(modifier.fillMaxWidth(),contentAlignment=Alignment.Center){Text(if(query.isBlank())"Нет доступных процессов"else"Ничего не найдено")};return}
 Box(modifier.fillMaxWidth()){
  val state=rememberLazyListState()
  LazyColumn(Modifier.fillMaxSize().padding(end=12.dp),state=state){items(entries,key={it.identity}){entry->
   Row(Modifier.fillMaxWidth().clickable(enabled=entry.selectable){onToggle(entry.path)}.padding(vertical=7.dp),verticalAlignment=Alignment.CenterVertically){
    Checkbox(entry.path in selected,null,enabled=entry.selectable)
    Column(Modifier.weight(1f).padding(start=4.dp)){
     Text(entry.name,fontSize=15.sp)
     Text(if(entry.path.startsWith("/"))entry.path else if(entry.uid==0L)"Системный процесс / поток ядра"else"Путь скрыт правами Linux · «Системные пути…»",fontSize=11.sp,fontFamily=FontFamily.Monospace,maxLines=2,overflow=TextOverflow.Ellipsis,color=LocalContentColor.current.copy(alpha=.65f))
     Text((if(entry.pids.isEmpty())"Не запущен · выбор сохранён"else"PID: "+entry.pids.joinToString(", "))+(if(entry.uid>=0)" · UID: ${entry.uid}"else"")+(if(entry.uid==0L)" · Уже в обход VPN"else""),fontSize=11.sp,color=LocalContentColor.current.copy(alpha=.55f))
    }
   }
   Divider(color=MaterialTheme.colors.onSurface.copy(alpha=.08f))
  }}
  VerticalScrollbar(rememberScrollbarAdapter(state),Modifier.align(Alignment.CenterEnd).fillMaxHeight())
 }
}
