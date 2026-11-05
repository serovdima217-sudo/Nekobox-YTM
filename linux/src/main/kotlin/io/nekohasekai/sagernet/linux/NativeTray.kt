package io.nekohasekai.sagernet.linux

import java.nio.file.*
import java.nio.file.attribute.PosixFilePermissions
import java.awt.image.BufferedImage
import javax.imageio.ImageIO
import javax.swing.SwingUtilities
import java.util.concurrent.*
import io.nekohasekai.sagernet.database.LinuxDatabase

/** Нативные StatusNotifierItem и DBusMenu, без моста XEmbed и его проблем в Plasma. */
internal class NativeTray(val show:()->Unit,val toggle:()->Unit,val quit:()->Unit,val toggleWindow:()->Unit):AutoCloseable {
 private val binary=Path.of(System.getenv("NEKOBOX_YTM_LIBEXEC")?:"bin").resolve("linux-tray")
 private var process:Process?=null
 private var writer:java.io.BufferedWriter?=null
 private var lastStatus=""
 @Volatile var service="";private set
 fun install():Boolean {
  if(System.getenv("DBUS_SESSION_BUS_ADDRESS").isNullOrBlank()||!Files.isExecutable(binary))return false
  return runCatching{
   val dir=LinuxDatabase.directory;Files.createDirectories(dir);Files.setPosixFilePermissions(dir,PosixFilePermissions.fromString("rwx------"))
   val original=ImageIO.read(NativeTray::class.java.getResourceAsStream("/android/icon/nekobox.png"))
   val image=BufferedImage(64,64,BufferedImage.TYPE_INT_ARGB);image.createGraphics().let{g->g.drawImage(original,0,0,64,64,null);g.dispose()}
   val pixels=ByteArray(64*64*4);for(y in 0..63)for(x in 0..63){val argb=image.getRGB(x,y);val i=(y*64+x)*4;for(k in 0..3)pixels[i+k]=(argb ushr (24-8*k)).toByte()}
   val icon=dir.resolve("tray-icon.argb");Files.write(icon,pixels);Files.setPosixFilePermissions(icon,PosixFilePermissions.fromString("rw-------"))
   val ready=CountDownLatch(1);val p=ProcessBuilder(binary.toAbsolutePath().toString(),icon.toString()).redirectError(dir.resolve("tray-native.log").toFile()).start();process=p;writer=p.outputStream.bufferedWriter()
   Thread({p.inputStream.bufferedReader().useLines{lines->lines.forEach{line->if(line.startsWith("готово\t")){service=line.substringAfter('\t');ready.countDown()}else SwingUtilities.invokeLater{when(line){"показать"->show();"окно"->toggleWindow();"подключение"->toggle();"выход"->quit()}}}};ready.countDown()},"События трея KDE").apply{isDaemon=true;start()}
   val ok=ready.await(5,TimeUnit.SECONDS)&&service.isNotBlank()&&p.isAlive
   if(!ok)close();ok
  }.getOrElse{close();false}
 }
 @Synchronized fun update(phase:String,selected:Boolean){
  val status=phase.replace('\n',' ').replace('\t',' ')+"\t"+if(selected)"1\n"else"0\n"
  if(status==lastStatus)return
  runCatching{writer?.apply{write(status);flush();lastStatus=status}}
 }

 fun probe(mode:String):String {
  check(service.isNotBlank());val p=ProcessBuilder(binary.toAbsolutePath().toString(),"--test",service,mode).redirectErrorStream(true).start()
  check(p.waitFor(5,TimeUnit.SECONDS)){p.destroyForcibly();"D-Bus не ответил"};val out=p.inputStream.bufferedReader().readText();check(p.exitValue()==0){"Ошибка настоящего меню D-Bus: $out"};return out
 }
 override fun close(){runCatching{writer?.apply{write("стоп\n");flush();close()}};process?.let{if(!it.waitFor(1,TimeUnit.SECONDS))it.destroyForcibly()};writer=null;process=null;service=""}
}
