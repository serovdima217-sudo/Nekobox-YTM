package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.animation.*
import androidx.compose.animation.core.*
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.material.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.*
import androidx.compose.ui.unit.*
import io.nekohasekai.sagernet.ktx.app

/** Отдельное состояние нижней проверки: результаты карточек TCP/URL не изменяются. */
data class CurrentConnectionTest(val profile:Long=0,val running:Boolean=false,val latency:Int?=null,val error:String?=null)
object AndroidTestPresentation {
 const val preparing="Установка соединения…"
 fun profileError(error:String?):String {
  val raw=error.orEmpty()
  if(listOf("connection_test_timeout","connection_test_refused","connection_test_domain_not_found","connection_test_unreachable","unavailable").any{raw==app.getString(it)})return raw
  val friendly=moe.matsuri.nb4a.Protocols.genFriendlyMsg(raw)
  if(friendly!=raw)return friendly
  val lower=raw.lowercase()
  return when {
   listOf("timed out","timeout","deadline").any{it in lower}->app.getString("connection_test_timeout")
   listOf("econnrefused","connection refused","closed pipe").any{it in lower}->app.getString("connection_test_refused")
   listOf("unknownhost","name or service not known","unresolved","nodename nor servname","name resolution","no address associated with hostname").any{it in lower}->app.getString("connection_test_domain_not_found")
   listOf("network is unreachable","no route to host","enetunreach").any{it in lower}->app.getString("connection_test_unreachable")
   else->app.getString("unavailable")
  }
 }
 fun bottomText(phase:String,test:CurrentConnectionTest,url:String):String = when {
  phase!="Подключено"->app.getString(if(phase=="Подключение…")"connecting"else if(phase=="Отключение…")"stopping"else"not_connected")
  test.running->preparing
  test.error!=null->app.getString("connection_test_error",profileError(test.error))
  test.latency!=null->app.getString(if(url.startsWith("https://"))"connection_test_available"else"connection_test_available_http",test.latency)
  else->app.getString("vpn_connected")
 }
}

/** Выемка под FAB: плавные плечи и дуга, а не прямоугольная панель за кнопкой. */
class AndroidCradleShape(private val fraction:Float=1f):Shape {
 override fun createOutline(size:Size,layoutDirection:LayoutDirection,density:Density):Outline {
  val k=density.density;val x=size.width/2f;val f=fraction.coerceIn(0f,1f)
  return Outline.Generic(Path().apply{
   moveTo(0f,0f);lineTo(x-44*k,0f)
   cubicTo(x-35*k,0f,x-35*k,5*k*f,x-30*k,13*k*f)
   cubicTo(x-17*k,36*k*f,x+17*k,36*k*f,x+30*k,13*k*f)
   cubicTo(x+35*k,5*k*f,x+35*k,0f,x+44*k,0f)
   lineTo(size.width,0f);lineTo(size.width,size.height);lineTo(0f,size.height);close()
  })
 }
}

@Composable fun AndroidConnectionBar(phase:String,tx:Long,rx:Long,result:CurrentConnectionTest,url:String,barColor:Color,primary:Color,progressColor:Color,test:()->Unit,toggle:()->Unit,modifier:Modifier=Modifier,ringFrame:Float?=null,onStatusLayout:(androidx.compose.ui.text.TextLayoutResult)->Unit={}) {
 val connected=phase=="Подключено"
 val header by animateDpAsState(if(connected)44.dp else 88.dp,tween(240,easing=FastOutSlowInEasing))
 val buttonTop by animateDpAsState(if(connected)10.dp else 16.dp,tween(240,easing=FastOutSlowInEasing))
 val cradle by animateFloatAsState(if(connected)1f else 0f,tween(240))
 Box(modifier.fillMaxWidth().animateContentSize(tween(240,easing=FastOutSlowInEasing))){
  Column {
   Spacer(Modifier.height(header))
   AnimatedVisibility(connected,enter=expandVertically(tween(240))+fadeIn(tween(200)),exit=shrinkVertically(tween(200))+fadeOut(tween(160))){
    Surface(Modifier.fillMaxWidth(),shape=AndroidCradleShape(cradle),color=barColor,elevation=8.dp){
     Column(Modifier.fillMaxWidth().clickable(enabled=connected&&!result.running,onClick=test).padding(horizontal=16.dp,vertical=16.dp)){
      Text("▲ ${bytes(tx)}/с",fontSize=14.sp,lineHeight=18.sp,color=Color.White)
      Text("▼ ${bytes(rx)}/с",fontSize=14.sp,lineHeight=18.sp,color=Color.White)
      AnimatedContent(AndroidTestPresentation.bottomText(phase,result,url),transitionSpec={fadeIn(tween(180)) togetherWith fadeOut(tween(120))}){text->
       Text(text,Modifier.fillMaxWidth().padding(top=2.dp),fontSize=16.sp,lineHeight=21.sp,color=Color.White,softWrap=true,onTextLayout=onStatusLayout)
      }
     }
    }
   }
  }
  Box(Modifier.align(Alignment.TopCenter).offset(y=buttonTop-8.dp).size(72.dp),contentAlignment=Alignment.Center){
   AndroidFabProgress(phase,progressColor,ringFrame)
   FloatingActionButton(onClick=toggle,backgroundColor=primary,contentColor=Color.White,elevation=FloatingActionButtonDefaults.elevation(6.dp)){
    AndroidServiceIcon(phase)
   }
  }
 }
}

@Composable fun AndroidFabProgress(phase:String,color:Color,frameOverride:Float?=null) {
 var visible by remember{mutableStateOf(false)}
 var completing by remember{mutableStateOf(false)}
 LaunchedEffect(phase){
  completing=false
  if(phase=="Подключение…") {visible=false;kotlinx.coroutines.delay(1200);visible=true}
  else if(phase=="Подключено"&&visible){completing=true;kotlinx.coroutines.delay(350);visible=false}
  else visible=false
 }
 if(frameOverride!=null||visible){
  if(completing)CircularProgressIndicator(progress=1f,modifier=Modifier.size(64.dp),color=color,strokeWidth=4.dp)
  else if(frameOverride!=null) {
   androidx.compose.foundation.Canvas(Modifier.size(64.dp)){
    drawArc(color,frameOverride*360f,230f,false,style=androidx.compose.ui.graphics.drawscope.Stroke(width=4.dp.toPx(),cap=StrokeCap.Round))
   }
  }else CircularProgressIndicator(Modifier.size(64.dp),color=color,strokeWidth=4.dp)
 }
}
