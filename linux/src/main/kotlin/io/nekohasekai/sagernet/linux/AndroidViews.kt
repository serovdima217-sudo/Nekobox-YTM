package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.*
import androidx.compose.material.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.graphics.*
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.graphics.drawscope.*
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.unit.dp
import org.w3c.dom.Element
import javax.xml.parsers.DocumentBuilderFactory

/** Ресурсы берутся из исходного дерева Android, включая имена действий и геометрию. */
object AndroidXml {
 fun root(kind:String,name:String):Element? = javaClass.getResourceAsStream("/android/$kind/$name.xml")?.use {
  val factory=DocumentBuilderFactory.newInstance();factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl",true)
  factory.newDocumentBuilder().parse(it).documentElement
 }
 fun children(e:Element):List<Element> = (0 until e.childNodes.length).mapNotNull{e.childNodes.item(it)as?Element}
 fun attr(e:Element,key:String)=e.getAttribute("android:$key").ifEmpty{e.getAttribute("app:$key")}
 fun id(value:String)=value.substringAfterLast('/')
 fun nodes(e:Element):List<Element> = listOf(e)+children(e).flatMap{nodes(it)}
 fun dimension(layout:String,id:String,attribute:String,fallback:Float):Float {
  val e=root("layout",layout)?.let{nodes(it).find{n->id(attr(n,"id"))==id}}?:return fallback
  return attr(e,attribute).removeSuffix("dp").removeSuffix("sp").toFloatOrNull()?:fallback
 }
}
data class AndroidMenuItem(val id:String,val title:String,val icon:String,val children:List<AndroidMenuItem>,val group:Int=0)
object AndroidMenus {
 fun load(name:String):List<AndroidMenuItem> {
  val root=AndroidXml.root("menu",name)?:return emptyList()
  fun walk(e:Element,group:Int):List<AndroidMenuItem> = AndroidXml.children(e).flatMap{n->
   when(n.tagName){
    "group"->walk(n,group+AndroidXml.children(e).indexOf(n)+1)
    "item"->listOf(AndroidMenuItem(AndroidXml.id(AndroidXml.attr(n,"id")),AndroidPreferences.text(AndroidXml.attr(n,"title")),AndroidXml.id(AndroidXml.attr(n,"icon")),AndroidXml.children(n).filter{it.tagName=="menu"}.flatMap{walk(it,group)},group))
    else->emptyList()
   }
  }
  return walk(root,0)
 }
}
private data class VectorPath(val path:Path,val clip:Boolean,val fill:Float)
/** Canvas заменяет только Android Canvas. Используются исходные pathData и clip-path. */
@Composable fun AndroidIcon(name:String,modifier:Modifier=Modifier.size(24.dp),tint:Color=LocalContentColor.current) {
 val vector=remember(name){
  var root=AndroidXml.root("drawable",name)
  if(root?.tagName=="animated-vector")root=AndroidXml.root("drawable",AndroidXml.id(AndroidXml.attr(root!!,"drawable")))
  val paths=root?.let{AndroidXml.nodes(it)}?.filter{it.tagName in listOf("path","clip-path")}?.mapNotNull{p->
   val data=AndroidXml.attr(p,"pathData")
   if(data.isBlank())null else VectorPath(PathParser().parsePathString(data).toPath().apply{if(AndroidXml.attr(p,"fillType")=="evenOdd")fillType=PathFillType.EvenOdd},p.tagName=="clip-path",AndroidXml.attr(p,"fillAlpha").toFloatOrNull()?:1f)
  }?:emptyList()
  Triple(root?.let{AndroidXml.attr(it,"viewportWidth").toFloatOrNull()}?:24f,root?.let{AndroidXml.attr(it,"viewportHeight").toFloatOrNull()}?:24f,paths)
 }
 Canvas(modifier){scale(size.width/vector.first,size.height/vector.second,pivot=Offset.Zero){
  var clip:Path?=null
  for(p in vector.third)if(p.clip)clip=p.path else if(clip==null)drawPath(p.path,tint,alpha=p.fill)else clipPath(clip!!){drawPath(p.path,tint,alpha=p.fill)}
 }}
}
@Composable fun AndroidAction(icon:String,label:String,onClick:()->Unit) {
 IconButton(onClick=onClick,modifier=Modifier.size(48.dp).semantics{contentDescription=label}){AndroidIcon(icon)}
}
@Composable fun AndroidPopup(items:List<AndroidMenuItem>,expanded:Boolean,dismiss:()->Unit,action:(String)->Unit) {
 var nested by remember(expanded){mutableStateOf<List<AndroidMenuItem>?>(null)}
 DropdownMenu(expanded,dismiss){
  if(nested!=null)DropdownMenuItem(onClick={nested=null}){Text("‹")}
  for(item in nested?:items)DropdownMenuItem(onClick={if(item.children.isNotEmpty())nested=item.children else{dismiss();action(item.id)}}){
   Text(item.title,Modifier.weight(1f));if(item.children.isNotEmpty())Text("›",Modifier.padding(start=20.dp))
  }
 }
}

@Composable fun androidTypography():Typography {
 val family=remember{androidx.compose.ui.text.font.FontFamily(
  androidx.compose.ui.text.platform.Font("android/fonts/Roboto-Regular.ttf",androidx.compose.ui.text.font.FontWeight.Normal),
  androidx.compose.ui.text.platform.Font("android/fonts/Roboto-Medium.ttf",androidx.compose.ui.text.font.FontWeight.Medium),
  androidx.compose.ui.text.platform.Font("android/fonts/Roboto-Bold.ttf",androidx.compose.ui.text.font.FontWeight.Bold)
 )}
 val base=Typography(defaultFontFamily=family)
 return base.copy(body1=base.body1.copy(letterSpacing=androidx.compose.ui.unit.TextUnit(0f,androidx.compose.ui.unit.TextUnitType.Sp)),body2=base.body2.copy(letterSpacing=androidx.compose.ui.unit.TextUnit(0f,androidx.compose.ui.unit.TextUnitType.Sp)))
}
