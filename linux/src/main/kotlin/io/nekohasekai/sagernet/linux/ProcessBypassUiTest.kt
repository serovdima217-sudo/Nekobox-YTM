@file:OptIn(androidx.compose.ui.ExperimentalComposeUiApi::class)
package io.nekohasekai.sagernet.linux

import androidx.compose.runtime.*
import androidx.compose.foundation.layout.*
import androidx.compose.material.*
import androidx.compose.ui.ImageComposeScene
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.input.pointer.PointerEventType
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import org.jetbrains.skia.EncodedImageFormat
import java.nio.file.*

object ProcessBypassUiTest {
 fun run() {
  val state=Path.of(System.getenv("NEKOBOX_LINUX_STATE")?:error("Нужна QA-база"))
  require(state.fileName.toString().startsWith("qa-"));Files.createDirectories(state)
  val rows=listOf(LinuxActiveProcess("/usr/bin/discord","Discord",listOf(421,435)),LinuxActiveProcess("/usr/bin/firefox","firefox",listOf(512,530)))
  var selected by mutableStateOf(emptySet<String>())
  val scene=ImageComposeScene(640,420,Density(1f)){
   MaterialTheme(typography=androidTypography()){Surface{Column(Modifier.fillMaxSize().padding(16.dp)){
    Text("Процессы без VPN",style=MaterialTheme.typography.h6)
    Text("Все процессы выбранного исполняемого файла идут напрямую",Modifier.padding(vertical=12.dp))
    LinuxProcessList(rows,selected,"",false,{p->selected=if(p in selected)selected-p else selected+p},Modifier.fillMaxWidth().weight(1f))
   }}}
  }
  try {
   scene.render().close()
   scene.sendPointerEvent(PointerEventType.Press,Offset(30f,112f));scene.sendPointerEvent(PointerEventType.Release,Offset(30f,112f));scene.render().close()
   check(selected==setOf("/usr/bin/discord")){"Строка процесса не обработала нажатие: $selected"}
   Thread.sleep(250);scene.render().close()
   scene.render().use{Files.write(state.resolve("выбор-процессов.png"),it.encodeToData(EncodedImageFormat.PNG)!!.bytes)}
   scene.sendPointerEvent(PointerEventType.Press,Offset(30f,112f));scene.sendPointerEvent(PointerEventType.Release,Offset(30f,112f));scene.render().close()
   check(selected.isEmpty())
   println("Интерфейс списка процессов: отрисовка, выбор и снятие реальным нажатием — успешно")
  }finally{scene.close()}
 }
}
