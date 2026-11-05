package io.nekohasekai.sagernet.linux
import androidx.compose.runtime.*
import androidx.compose.foundation.layout.*
import androidx.compose.material.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.awt.SwingPanel
import javafx.application.Platform
import javafx.embed.swing.JFXPanel
import javafx.scene.Scene
import javafx.scene.web.WebView
import com.sun.net.httpserver.HttpServer
import java.net.*
import java.util.zip.ZipInputStream
import java.util.concurrent.Executors
import java.awt.GraphicsEnvironment
import java.nio.file.*

/** Включена именно yacd.zip из Android APK. Секрет API остаётся в локальном URL. */
object AndroidDashboard {
 private val files by lazy {
  val out=mutableMapOf<String,ByteArray>()
  ZipInputStream(javaClass.getResourceAsStream("/android/assets/yacd.zip")!!).use{zip->while(true){val e=zip.nextEntry?:break;if(!e.isDirectory){val name=e.name.substringAfter('/');require(!name.split('/').contains(".."));out[name]=zip.readBytes()}}};out
 }
 private val server by lazy {
  HttpServer.create(InetSocketAddress("127.0.0.1",0),0).apply{
   executor=Executors.newCachedThreadPool{r->Thread(r,"Локальная панель").apply{isDaemon=true}}
   createContext("/"){e->if(NativeWebSockets.handle(e))return@createContext
    val path=e.requestURI.path.removePrefix("/").ifBlank{"index.html"};val original=files[path];val data=if(path=="index.html"&&original!=null)String(original).replace("<head>","<head><script src=\"/linux-websocket.js\"></script>").toByteArray()else if(path=="linux-websocket.js")AndroidDashboard::class.java.getResourceAsStream("/android/assets/linux-websocket.js")!!.use{it.readBytes()}else original
    if(data==null){e.sendResponseHeaders(404,-1);e.close()}else{
     e.responseHeaders.set("Content-Type",when(path.substringAfterLast('.')){"js"->"application/javascript";"css"->"text/css";"html"->"text/html; charset=utf-8";"json","webmanifest"->"application/json";"png"->"image/png";"svg"->"image/svg+xml";else->"application/octet-stream"})
     e.responseHeaders.set("Cache-Control","no-store");e.responseHeaders.set("Referrer-Policy","no-referrer");e.sendResponseHeaders(200,data.size.toLong());e.responseBody.use{it.write(data)}
    }
   };start()
  }
 }
 fun url():String="http://127.0.0.1:${server.address.port}/"
 fun asset(path:String):ByteArray?=files[path]
 fun stop(){NativeWebSockets.close();runCatching{server.stop(0)};if(LinuxWebView.started)Platform.exit()}
}
object LinuxWebView {@Volatile var started=false}
@Composable fun AndroidDashboardView(url:String,modifier:Modifier=Modifier){
 if(GraphicsEnvironment.isHeadless()){Column(modifier){Text("Панель sing-box — исходная yacd Android");Text("WebKit проверяется отдельно в графической сессии.")};return}
 var failure by remember(url){mutableStateOf("")}
 val panel=remember{JFXPanel().also{LinuxWebView.started=true;Platform.setImplicitExit(false)}}
 var engine by remember{mutableStateOf<javafx.scene.web.WebEngine?>(null)}
 DisposableEffect(panel){Platform.runLater{try{val view=WebView();engine=view.engine;panel.scene=Scene(view);view.engine.isJavaScriptEnabled=true;view.engine.userDataDirectory=LinuxDatabasePath.webData.toFile();view.engine.load(url)}catch(t:Throwable){failure=t.message?:"WebKit не запустился"}};onDispose{Platform.runLater{engine?.load(null);panel.scene=null}}}
 LaunchedEffect(url){Platform.runLater{engine?.load(url)}}
 if(failure.isNotBlank())Text(failure,modifier,color=MaterialTheme.colors.error)else SwingPanel(factory={panel},modifier=modifier)
}
private object LinuxDatabasePath {val webData by lazy{io.nekohasekai.sagernet.database.LinuxDatabase.directory.resolve("webview").also{Files.createDirectories(it)}}}
