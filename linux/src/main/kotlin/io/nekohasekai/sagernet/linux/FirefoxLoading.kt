package io.nekohasekai.sagernet.linux
import androidx.compose.runtime.*
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.*
import androidx.compose.material.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.Alignment
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** Исходные pathData и интервалы animation-list Android/Mozilla, MPL-2.0. */
object FirefoxFrames {
 val frames=AndroidXml.children(AndroidXml.root("drawable","tm_firefox_loading")!!).map{item->
  val root=AndroidXml.root("drawable",AndroidXml.id(AndroidXml.attr(item,"drawable")))!!
  val path=AndroidXml.nodes(root).first{it.tagName=="path"}
  Triple(PathParser().parsePathString(AndroidXml.attr(path,"pathData")).toPath(),AndroidXml.attr(item,"duration").toInt(),Color(AndroidXml.attr(path,"fillColor").removePrefix("#").toLong(16)or 0xFF000000L))
 }
 val duration=frames.sumOf{it.second}
 fun frameAt(ms:Long):Int{var t=(ms%duration).toInt();for(i in frames.indices){if(t<frames[i].second)return i;t-=frames[i].second};return 0}
}
@Composable fun FirefoxLoading(label:String,vertical:Boolean=false,frameOverride:Int?=null){
 var frame by remember{mutableIntStateOf(0)}
 LaunchedEffect(frameOverride){if(frameOverride==null){val start=withFrameNanos{it};while(true)withFrameNanos{frame=FirefoxFrames.frameAt((it-start)/1_000_000)}}}
 val selected=FirefoxFrames.frames[frameOverride?.coerceIn(0,FirefoxFrames.frames.lastIndex)?:frame]
 @Composable fun Dots(size:Int){Canvas(Modifier.size(size.dp)){scale(this.size.width/16f,this.size.height/16f,pivot=androidx.compose.ui.geometry.Offset.Zero){drawPath(selected.first,selected.third)}}}
 if(vertical)Column(Modifier.widthIn(min=260.dp).heightIn(min=176.dp).padding(24.dp),horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.Center){Dots(64);Text(label,Modifier.padding(top=12.dp),fontSize=16.sp)}
 else Row(Modifier.fillMaxWidth().padding(8.dp),horizontalArrangement=Arrangement.Center,verticalAlignment=Alignment.CenterVertically){Dots(32);Text(label,Modifier.padding(start=10.dp),style=MaterialTheme.typography.body2)}
}
