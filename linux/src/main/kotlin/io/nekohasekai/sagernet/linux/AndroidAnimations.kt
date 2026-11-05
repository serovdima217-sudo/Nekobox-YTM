package io.nekohasekai.sagernet.linux
import androidx.compose.runtime.*
import androidx.compose.animation.core.*
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.size
import androidx.compose.material.LocalContentColor
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.graphics.drawscope.*
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.unit.dp

/** Морфинг исходных strike_thru_path/strike_thru_mask из Android animated-vector. */
object AndroidServicePaths {
 private val pattern=Regex("-?\\d+(?:\\.\\d+)?")
 val animations=listOf("ic_service_connecting","ic_service_connected","ic_service_stopping","ic_service_stopped").associateWith{name->
  AndroidXml.nodes(AndroidXml.root("drawable",name)!!).filter{it.tagName=="target"}.associate{target->
   val anim=AndroidXml.nodes(target).first{it.tagName=="objectAnimator"}
   AndroidXml.attr(target,"name") to (AndroidXml.attr(anim,"valueFrom") to AndroidXml.attr(anim,"valueTo"))
  }
 }
 val morphs=animations.getValue("ic_service_connecting")
 val filling=animations.getValue("ic_service_connected").getValue("path")
 fun interpolate(from:String,to:String,t:Float):String {
  val a=pattern.findAll(from).map{it.value.toFloat()}.toList();val b=pattern.findAll(to).map{it.value.toFloat()}.toList();require(a.size==b.size)
  var i=0;return pattern.replace(from){val n=i++;(a[n]+(b[n]-a[n])*t.coerceIn(0f,1f)).toString()}
 }
}
@Composable fun AndroidServiceIcon(phase:String){
 // Четыре состояния Android: убрать перечёркивание, заполнить самолёт,
 // открыть отверстия при остановке, затем вернуть перечёркивание.
 val progress by animateFloatAsState(if(phase!="Отключено")1f else 0f,tween(200,easing=FastOutSlowInEasing))
 val filling by animateFloatAsState(if(phase=="Подключено")1f else 0f,tween(200,easing=FastOutSlowInEasing))
 val color=LocalContentColor.current
 val paths=remember(progress,filling){AndroidXml.nodes(AndroidXml.root("drawable","ic_service_idle")!!).filter{it.tagName in listOf("path","clip-path")}.map{p->
  val morph=AndroidServicePaths.morphs[AndroidXml.attr(p,"name")];val data=if(AndroidXml.attr(p,"name")=="holey_icon")AndroidServicePaths.filling.let{AndroidServicePaths.interpolate(it.first,it.second,filling)}else if(morph!=null)AndroidServicePaths.interpolate(morph.first,morph.second,progress)else AndroidXml.attr(p,"pathData")
  p.tagName to PathParser().parsePathString(data).toPath()
 }}
 Canvas(Modifier.size(24.dp)){scale(size.width/24f,size.height/24f,pivot=Offset.Zero){var clip:Path?=null;for((type,path)in paths)if(type=="clip-path")clip=path else if(clip==null)drawPath(path,color)else clipPath(clip!!){drawPath(path,color)}}}
}
