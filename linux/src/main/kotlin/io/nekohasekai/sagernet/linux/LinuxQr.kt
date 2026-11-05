package io.nekohasekai.sagernet.linux

import com.google.zxing.*
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.qrcode.QRCodeWriter
import com.google.zxing.multi.qrcode.QRCodeMultiReader
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel
import java.awt.Image
import java.awt.Toolkit
import java.awt.datatransfer.*
import java.awt.image.BufferedImage
import java.io.ByteArrayOutputStream
import java.nio.file.*
import java.util.concurrent.TimeUnit
import javax.imageio.ImageIO
import kotlinx.coroutines.*

/** Все ссылки и изображения обрабатываются локально, без внешнего QR-сервиса. */
object LinuxQr {
 fun encode(text:String):BufferedImage {
  require(text.isNotBlank()){"Нечего кодировать"}
  require(text.toByteArray().size<=2953){"Ссылка слишком велика для одного QR-кода. Сохраните конфигурацию в файл; данные не сокращались."}
  val matrix=try{QRCodeWriter().encode(text,BarcodeFormat.QR_CODE,0,0,mapOf(EncodeHintType.CHARACTER_SET to "UTF-8",EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.M,EncodeHintType.MARGIN to 4))}catch(_:WriterException){error("Данные не помещаются в один QR-код. Используйте экспорт файла.")}
  val scale=maxOf(4,768/matrix.width);val size=matrix.width*scale
  return BufferedImage(size,size,BufferedImage.TYPE_INT_RGB).apply{
   for(y in 0 until size)for(x in 0 until size)setRGB(x,y,if(matrix[x/scale,y/scale])0x000000 else 0xFFFFFF)
  }
 }
 fun png(image:BufferedImage):ByteArray=ByteArrayOutputStream().use{check(ImageIO.write(image,"png",it));it.toByteArray()}
 fun save(path:Path,image:BufferedImage){
  val absolute=path.toAbsolutePath();val temporary=Files.createTempFile(absolute.parent,".nekobox-qr-",".png")
  try{Files.write(temporary,png(image));Files.move(temporary,absolute,StandardCopyOption.REPLACE_EXISTING,StandardCopyOption.ATOMIC_MOVE)}finally{Files.deleteIfExists(temporary)}
 }
 fun decode(image:BufferedImage):List<String> {
  require(image.width.toLong()*image.height<=25_000_000L){"Слишком большое изображение (максимум 25 мегапикселей)"}
  val rgb=image.getRGB(0,0,image.width,image.height,null,0,image.width)
  val source=RGBLuminanceSource(image.width,image.height,rgb)
  val hints=mapOf(DecodeHintType.TRY_HARDER to true,DecodeHintType.CHARACTER_SET to "UTF-8",DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE))
  for(candidate in listOf<LuminanceSource>(source,source.invert())){
   val bitmap=BinaryBitmap(HybridBinarizer(candidate))
   val oriented=bitmap
   val multi=runCatching{QRCodeMultiReader().decodeMultiple(oriented,hints).map{it.text}.distinct()}.getOrDefault(emptyList())
   if(multi.isNotEmpty())return multi
   runCatching{MultiFormatReader().decode(oriented,hints).text}.getOrNull()?.let{return listOf(it)}
  }
  error("QR-код не найден. Выберите более чёткое изображение с белыми полями вокруг кода.")
 }
 fun read(path:Path):BufferedImage {
  require(Files.size(path)<=32*1024*1024){"Файл изображения слишком велик"}
  return ImageIO.createImageInputStream(path.toFile()).use{input->
   val readers=ImageIO.getImageReaders(input);require(readers.hasNext()){"Формат изображения не распознан"}
   val reader=readers.next()
   try{reader.input=input;require(reader.getWidth(0).toLong()*reader.getHeight(0)<=25_000_000L){"Слишком большое изображение"};reader.read(0)}finally{reader.dispose()}
  }
 }
 fun copy(image:BufferedImage){Toolkit.getDefaultToolkit().systemClipboard.setContents(object:Transferable{
  override fun getTransferDataFlavors()=arrayOf(DataFlavor.imageFlavor)
  override fun isDataFlavorSupported(flavor:DataFlavor)=flavor==DataFlavor.imageFlavor
  override fun getTransferData(flavor:DataFlavor):Any {if(!isDataFlavorSupported(flavor))throw UnsupportedFlavorException(flavor);return image}
 },null)}
 fun clipboard():List<String> {
  val clipboard=Toolkit.getDefaultToolkit().systemClipboard
  if(clipboard.isDataFlavorAvailable(DataFlavor.imageFlavor)){
   val image=clipboard.getData(DataFlavor.imageFlavor)as Image
   val w=image.getWidth(null);val h=image.getHeight(null);require(w>0&&h>0&&w.toLong()*h<=25_000_000L){"Недопустимый размер изображения"}
   val buffered=BufferedImage(w,h,BufferedImage.TYPE_INT_RGB);buffered.createGraphics().let{g->try{g.drawImage(image,0,0,null)}finally{g.dispose()}}
   return decode(buffered)
  }
  if(clipboard.isDataFlavorAvailable(DataFlavor.stringFlavor))return listOf(clipboard.getData(DataFlavor.stringFlavor).toString())
  error("В буфере нет изображения или ссылки")
 }
 fun executable(name:String):String?=System.getenv("PATH").orEmpty().split(':').map{Path.of(it,name)}.firstOrNull{Files.isRegularFile(it)&&Files.isExecutable(it)}?.toString()
 suspend fun capture(device:Path?=null):BufferedImage=runInterruptible(Dispatchers.IO){
  val image=Files.createTempFile("nekobox-qr-capture-",".png");val log=Files.createTempFile("nekobox-qr-capture-",".log")
  var process:Process?=null
  try{
   val args=if(device!=null){
    require(device.parent==Path.of("/dev")&&device.fileName.toString().matches(Regex("video[0-9]+"))){"Недопустимое устройство камеры"}
    listOf(executable("ffmpeg")?:error("Для камеры установите ffmpeg"),"-nostdin","-loglevel","error","-f","v4l2","-i",device.toString(),"-frames:v","1","-vf","scale='min(1280,iw)':-2","-y",image.toString())
   }else when {
    executable("spectacle")!=null->listOf(executable("spectacle")!!,"--background","--nonotify","--region","--output",image.toString())
    executable("gnome-screenshot")!=null->listOf(executable("gnome-screenshot")!!,"-a","-f",image.toString())
    else->error("Выберите файл снимка экрана: интерактивная съёмка требует Spectacle или gnome-screenshot")
   }
   process=ProcessBuilder(args).redirectError(log.toFile()).redirectOutput(ProcessBuilder.Redirect.DISCARD).start()
   check(process.waitFor(if(device==null)120 else 12,TimeUnit.SECONDS)){"Истекло время ожидания изображения"}
   check(process.exitValue()==0&&Files.size(image)>0){if(device==null)"Съёмка отменена или не разрешена системой"else"Не удалось открыть камеру. Проверьте разрешения и не занята ли она другим приложением."}
   read(image)
  }finally{process?.let{if(it.isAlive)it.destroyForcibly()};Files.deleteIfExists(image);Files.deleteIfExists(log)}
 }
 fun cameras():List<Path> = Files.list(Path.of("/dev")).use{it.filter{p->p.fileName.toString().matches(Regex("video[0-9]+"))}.sorted().toList()}
}
