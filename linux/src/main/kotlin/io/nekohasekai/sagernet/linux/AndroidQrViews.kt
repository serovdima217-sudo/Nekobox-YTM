package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.material.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.FilterQuality
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.*
import java.awt.image.BufferedImage

@Composable fun AndroidQrContent(image:BufferedImage,title:String,modifier:Modifier=Modifier) {
 val bitmap=remember(image){androidx.compose.ui.res.loadImageBitmap(java.io.ByteArrayInputStream(LinuxQr.png(image)))}
 Column(modifier,horizontalAlignment=Alignment.CenterHorizontally){
  Text(title,style=MaterialTheme.typography.h6)
  Box(Modifier.padding(vertical=16.dp).sizeIn(maxWidth=384.dp,maxHeight=384.dp).aspectRatio(1f).background(Color.White).padding(8.dp)){
   Image(bitmap,"QR-код",Modifier.fillMaxSize(),filterQuality=FilterQuality.None)
  }
  Text("Код содержит данные доступа. Передавайте его только доверенным людям.",fontSize=12.sp,color=LocalContentColor.current.copy(alpha=.65f))
 }
}
@Composable fun AndroidQrDialog(text:String,title:String,dismiss:()->Unit) {
 var image by remember(text){mutableStateOf<BufferedImage?>(null)}
 var error by remember{mutableStateOf("")}
 LaunchedEffect(text){try{image=withContext(Dispatchers.Default){LinuxQr.encode(text)}}catch(t:Throwable){if(t is CancellationException)throw t;error=t.message?:"Не удалось создать QR-код"}}
 AlertDialog(onDismissRequest=dismiss,modifier=Modifier.widthIn(max=520.dp),title={},text={
  Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState())){
   image?.let{AndroidQrContent(it,title,Modifier.fillMaxWidth())}?:run{if(error.isBlank())CircularProgressIndicator()}
   if(error.isNotBlank())Text(error,color=MaterialTheme.colors.error)
  }
 },confirmButton={Row{
  TextButton(onClick={try{image?.let{LinuxQr.copy(it)}}catch(t:Throwable){error=t.message?:"Буфер недоступен"}},enabled=image!=null){Text("Копировать")}
  TextButton(onClick={chooseFile(true)?.let{f->try{image?.let{LinuxQr.save((if(f.extension.equals("png",true))f else java.io.File(f.parentFile,f.name+".png")).toPath(),it)}}catch(t:Throwable){error=t.message?:"Не удалось сохранить изображение"}}},enabled=image!=null){Text("Сохранить PNG")}
 }},dismissButton={TextButton(onClick=dismiss){Text("Закрыть")}})
}
@Composable fun AndroidQrScanner(dismiss:()->Unit,accept:(String)->Unit) {
 val scope=rememberCoroutineScope();var error by remember{mutableStateOf("")};var busy by remember{mutableStateOf(false)}
 var preview by remember{mutableStateOf<BufferedImage?>(null)};var camera by remember{mutableStateOf(false)}
 val devices=remember{LinuxQr.cameras()};var device by remember{mutableStateOf(devices.firstOrNull())}
 fun scan(block:suspend()->List<String>){scope.launch{busy=true;error="";try{val results=block();require(results.isNotEmpty());accept(results.distinct().joinToString("\n"))}catch(t:Throwable){if(t is CancellationException)throw t;error=t.message?:"Не удалось прочитать QR-код"}finally{busy=false}}}
 LaunchedEffect(camera,device){
  if(camera&&device!=null)try{while(isActive){
   busy=true
   val frame=LinuxQr.capture(device);preview=frame
   val results=withContext(Dispatchers.Default){runCatching{LinuxQr.decode(frame)}.getOrNull()}
   busy=false
   if(!results.isNullOrEmpty()){camera=false;accept(results.distinct().joinToString("\n"));break}
   delay(350)
  }}catch(t:Throwable){if(t is CancellationException)throw t;error=t.message?:"Ошибка камеры";camera=false}finally{busy=false}
 }
 AlertDialog(onDismissRequest=dismiss,modifier=Modifier.widthIn(max=540.dp),title={Text("Сканировать QR-код")},text={
  Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState())){
   Text("Изображения и ссылки распознаются только на этом компьютере.",fontSize=13.sp)
   preview?.let{image->val bitmap=remember(image){androidx.compose.ui.res.loadImageBitmap(java.io.ByteArrayInputStream(LinuxQr.png(image)))};Image(bitmap,"Изображение камеры",Modifier.fillMaxWidth().heightIn(max=240.dp).padding(vertical=8.dp))}
   Button(onClick={chooseFile(false)?.let{f->scan{withContext(Dispatchers.IO){LinuxQr.decode(LinuxQr.read(f.toPath()))}}}},modifier=Modifier.fillMaxWidth(),enabled=!busy&&!camera){Text("Изображение из файла")}
   Button(onClick={scan{withContext(Dispatchers.IO){LinuxQr.clipboard()}}},modifier=Modifier.fillMaxWidth(),enabled=!busy&&!camera){Text("Из буфера обмена")}
   Button(onClick={scan{val frame=LinuxQr.capture();withContext(Dispatchers.Default){LinuxQr.decode(frame)}}},modifier=Modifier.fillMaxWidth(),enabled=!busy&&!camera){Text("Выбрать область экрана")}
   if(devices.isNotEmpty()){
    if(devices.size>1)Row{devices.forEach{p->TextButton(onClick={device=p},enabled=!camera){Text(p.fileName.toString())}}}
    Button(onClick={camera=!camera;error=""},modifier=Modifier.fillMaxWidth()){Text(if(camera)"Остановить камеру"else"Включить камеру")}
   }else Text("Камера не обнаружена. Доступен импорт изображения или снимка экрана.",Modifier.padding(top=8.dp),fontSize=12.sp)
   if(busy)LinearProgressIndicator(Modifier.fillMaxWidth().padding(top=8.dp))
   if(error.isNotBlank())Text(error,Modifier.padding(top=8.dp),color=MaterialTheme.colors.error)
  }
 },confirmButton={},dismissButton={TextButton(onClick=dismiss){Text("Отмена")}})
}
